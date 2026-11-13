package resourceutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// defaultTimeout is used when providerdata.Data.Timeout is zero.
const defaultTimeout = 5 * time.Minute

// KindResource is implemented by every resource package's model struct.
//
// Model structs are populated by the framework with reflection, which rebuilds
// the struct from the schema: only exported fields with a tfsdk tag survive a
// Plan.Get or State.Get. Do not keep state that the harness needs in
// unexported fields.
type KindResource interface {
	// Kind returns the config resource kind.
	Kind() configresource.Kind
	// ID returns the desired id derived from the model (spec §7.0). It returns
	// "" when the server generates the id (llm.apiKey).
	ID() string
	// Value marshals the model to the API value object.
	Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage
	// SetValue unmarshals an API value into the model, preserving attributes
	// the server did not return.
	SetValue(ctx context.Context, value json.RawMessage, diags *diag.Diagnostics)
	// SetServerMeta stores the computed id and revision returned by the server.
	SetServerMeta(id string, revision int64)
}

// Spec describes a resource kind for the shared CRUD harness.
type Spec struct {
	TypeName string
	Kind     configresource.Kind
	Schema   schema.Schema
	// New returns a fresh model. See KindResource for the reflection rules that
	// apply to the returned struct.
	New func() KindResource
}

// CRUD returns a resource.Resource that drives Create/Read/Update/Delete for
// the given kind specification.
func CRUD(spec Spec) resource.Resource {
	return &kindResource{spec: spec}
}

// kindResource is the shared resource implementation.
type kindResource struct {
	spec Spec
	data *providerdata.Data
}

// Metadata returns the Terraform type name.
func (r *kindResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = r.spec.TypeName
}

// Schema returns the resource schema.
func (r *kindResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = r.spec.Schema
}

// Configure stores the provider data for later operations.
func (r *kindResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected *providerdata.Data, got %T", req.ProviderData),
		)
		return
	}
	r.data = data
}

// Create writes a new resource, refusing to adopt an existing id.
func (r *kindResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	data := r.data
	if data == nil {
		resp.Diagnostics.AddError("Provider not configured", "resource received no provider data")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	model := r.spec.New()
	if diags := req.Plan.Get(ctx, model); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	if data.StoreMode == "file" && r.spec.Kind.IsSingleton() {
		resp.Diagnostics.AddError(
			fmt.Sprintf("%s unavailable in file mode", r.spec.TypeName),
			fmt.Sprintf("%s requires config.storage.mode = hybrid; the gateway cannot read this resource back in file mode", r.spec.TypeName),
		)
		return
	}

	id := model.ID()

	// Server-generated ids (llm.apiKey) are unknown before the first write, so
	// there is nothing to check against. Every other kind derives its id from
	// the plan and must not adopt an existing out-of-band resource.
	if id != "" {
		list, err := data.Client.List(ctx, r.spec.Kind)
		if err != nil {
			r.addClientError(&resp.Diagnostics, err, "list", id)
			return
		}
		for _, res := range list {
			if res.ID == id {
				resp.Diagnostics.AddError(
					fmt.Sprintf("%s already exists", r.spec.TypeName),
					fmt.Sprintf("%s/%s already exists outside Terraform state; import it with terraform import <address> %s/%s", r.spec.Kind, id, r.spec.Kind, id),
				)
				return
			}
		}
	}

	value := model.Value(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "creating resource", map[string]any{
		"kind": r.spec.Kind,
		"id":   id,
	})

	var (
		res configresource.Resource
		err error
	)
	if r.spec.Kind.IsPolicy() {
		res, err = data.Client.Update(ctx, r.spec.Kind, id, value)
	} else {
		res, err = data.Client.Create(ctx, r.spec.Kind, value)
	}
	if err != nil {
		r.addClientError(&resp.Diagnostics, err, "create", id)
		return
	}

	model.SetValue(ctx, res.Value, &resp.Diagnostics)
	model.SetServerMeta(res.ID, res.Revision)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// Read refreshes the resource from the server, removing it from state when
// missing.
func (r *kindResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	data := r.data
	if data == nil {
		resp.Diagnostics.AddError("Provider not configured", "resource received no provider data")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	id, diags := stateID(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	list, err := data.Client.List(ctx, r.spec.Kind)
	if err != nil {
		r.addClientError(&resp.Diagnostics, err, "read", id)
		return
	}

	for _, res := range list {
		if res.ID != id {
			continue
		}
		model := r.spec.New()
		if diags := req.State.Get(ctx, model); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		model.SetValue(ctx, res.Value, &resp.Diagnostics)
		model.SetServerMeta(res.ID, res.Revision)
		resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
		return
	}

	resp.State.RemoveResource(ctx)
}

// Update writes changes to the server, storing the returned id to support
// rename.
func (r *kindResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	data := r.data
	if data == nil {
		resp.Diagnostics.AddError("Provider not configured", "resource received no provider data")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	oldID, diags := stateID(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	model := r.spec.New()
	if diags := req.Plan.Get(ctx, model); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	value := model.Value(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "updating resource", map[string]any{
		"kind":   r.spec.Kind,
		"old_id": oldID,
	})

	res, err := data.Client.Update(ctx, r.spec.Kind, oldID, value)
	if err != nil {
		r.addClientError(&resp.Diagnostics, err, "update", oldID)
		return
	}

	model.SetValue(ctx, res.Value, &resp.Diagnostics)
	model.SetServerMeta(res.ID, res.Revision)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// Delete removes the resource. A missing resource is treated as success.
func (r *kindResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	data := r.data
	if data == nil {
		resp.Diagnostics.AddError("Provider not configured", "resource received no provider data")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	id, diags := stateID(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	tflog.Debug(ctx, "deleting resource", map[string]any{
		"kind": r.spec.Kind,
		"id":   id,
	})

	err := data.Client.Delete(ctx, r.spec.Kind, id)
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		r.addClientError(&resp.Diagnostics, err, "delete", id)
		return
	}

	resp.State.RemoveResource(ctx)
}

// ImportState parses the import id and stores it in state.
func (r *kindResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := ParseImportID(r.spec.Kind, req.ID)
	if err != nil {
		resp.Diagnostics.AddError(r.errTitle("import"), err.Error())
		return
	}

	model := r.spec.New()
	model.SetServerMeta(id, 0)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// timeout returns the configured timeout or a default.
func timeout(d time.Duration) time.Duration {
	if d == 0 {
		return defaultTimeout
	}
	return d
}

// addClientError converts a client error into an actionable diagnostic.
func (r *kindResource) addClientError(diags *diag.Diagnostics, err error, operation, id string) {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch {
		case errors.Is(err, client.ErrNotFound):
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "resource not found; run terraform refresh to update state", apiErr.Message),
			)
		case errors.Is(err, client.ErrReadOnly):
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "set config.storage.mode to hybrid (or file) and restart the gateway", apiErr.Message),
			)
		case errors.Is(err, client.ErrConflict):
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "conflict detected; see the gateway message below", apiErr.Message),
			)
		case errors.Is(err, client.ErrInvalid):
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "the request was invalid; see the gateway message below", apiErr.Message),
			)
		case errors.Is(err, client.ErrTransient):
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "transient failure; retry the operation", apiErr.Message),
			)
		case errors.Is(err, client.ErrServer):
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "gateway error; see the gateway message below", apiErr.Message),
			)
		default:
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "unexpected error", apiErr.Message),
			)
		}
		return
	}

	diags.AddError(
		r.errTitle(operation),
		r.errDetail(operation, id, "unexpected error", err.Error()),
	)
}

func (r *kindResource) errTitle(operation string) string {
	return fmt.Sprintf("%s %s failed", r.spec.TypeName, operation)
}

func (r *kindResource) errDetail(operation, id, action, serverMsg string) string {
	if id == "" {
		return fmt.Sprintf("%s for %s. %s.", operation, r.spec.Kind, action)
	}
	if serverMsg == "" {
		return fmt.Sprintf("%s %s/%s failed. %s.", operation, r.spec.Kind, id, action)
	}
	return fmt.Sprintf("%s %s/%s failed. %s. Gateway message: %s", operation, r.spec.Kind, id, action, serverMsg)
}
