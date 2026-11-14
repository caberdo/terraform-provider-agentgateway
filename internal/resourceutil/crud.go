package resourceutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// defaultTimeout matches the provider's default timeout (spec §6).
const defaultTimeout = 30 * time.Second

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

	if !r.checkStoreMode(&resp.Diagnostics) {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	model := r.spec.New()
	if diags := req.Plan.Get(ctx, model); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	id := model.ID()
	if id == "" {
		// llm.apiKey is the only kind whose id is assigned by the server; it is
		// written to the collection and cannot be adopted by accident. Every
		// other kind must derive an id, otherwise the pre-flight adoption check
		// would be silently disabled.
		if r.spec.Kind != configresource.KindLLMAPIKey {
			resp.Diagnostics.AddError(
				fmt.Sprintf("%s has no id", r.spec.TypeName),
				fmt.Sprintf("%s create requires an id derived from the plan; set the attribute that determines it (for policies, name) and re-apply", r.spec.TypeName),
			)
			return
		}
	} else {
		list, err := data.Client.List(ctx, r.spec.Kind)
		if err != nil {
			r.addClientError(&resp.Diagnostics, err, "create", id)
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

	if !r.checkStoreMode(&resp.Diagnostics) {
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

	if !r.checkStoreMode(&resp.Diagnostics) {
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
		"kind": r.spec.Kind,
		"id":   oldID,
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

	// A singleton only exists after its first apply (spec §7.0). Importing one
	// before that would store an id that Read can never resolve, so check the
	// list first and say so. File mode is rejected for the same reason as Read.
	if r.spec.Kind.IsSingleton() {
		if r.data == nil {
			resp.Diagnostics.AddError("Provider not configured", "resource received no provider data")
			return
		}
		if !r.checkStoreMode(&resp.Diagnostics) {
			return
		}

		ctx, cancel := context.WithTimeout(ctx, timeout(r.data.Timeout))
		defer cancel()

		list, err := r.data.Client.List(ctx, r.spec.Kind)
		if err != nil {
			r.addClientError(&resp.Diagnostics, err, "import", id)
			return
		}
		found := false
		for _, res := range list {
			if res.ID == id {
				found = true
				break
			}
		}
		if !found {
			resp.Diagnostics.AddError(
				r.errTitle("import"),
				fmt.Sprintf("%s/%s does not exist yet; singletons only exist after their first apply, so apply the resource before importing it", r.spec.Kind, id),
			)
			return
		}
	}

	model := r.spec.New()
	model.SetServerMeta(id, 0)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

// checkStoreMode rejects a kind the gateway cannot read back in the current
// storage mode. In file mode the list omits singletons (C24), so letting the
// operation through would silently drop the resource from state.
func (r *kindResource) checkStoreMode(diags *diag.Diagnostics) bool {
	if r.data.StoreMode == "file" && r.spec.Kind.IsSingleton() {
		diags.AddError(
			fmt.Sprintf("%s unavailable in file mode", r.spec.TypeName),
			fmt.Sprintf("%s requires config.storage.mode = hybrid; the gateway cannot read this resource back in file mode", r.spec.TypeName),
		)
		return false
	}
	return true
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
			// The gateway retries generation conflicts internally up to 3 times;
			// a residual 409 is surfaced here and advises a re-apply.
			diags.AddError(
				r.errTitle(operation),
				r.errDetail(operation, id, "conflict detected; see the gateway message below", apiErr.Message),
			)
		case errors.Is(err, client.ErrInvalid):
			if attribute := r.invalidAttribute(apiErr.Message); attribute != "" {
				detail := fmt.Sprintf("the request was invalid; correct the %s attribute and re-apply", attribute)
				if apiErr.Message != "" {
					detail = fmt.Sprintf("%s. Correct the %s attribute and re-apply.", strings.TrimSuffix(apiErr.Message, "."), attribute)
				}
				diags.AddAttributeError(path.Root(attribute), r.errTitle(operation), detail)
				return
			}
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

// errDetail builds the actionable detail sentence. The server message is
// included whenever the gateway sent one, including for kinds whose id is
// assigned by the server (llm.apiKey), where there is no id to name.
func (r *kindResource) errDetail(operation, id, action, serverMsg string) string {
	var detail string
	if id == "" {
		detail = fmt.Sprintf("%s for %s. %s.", operation, r.spec.Kind, action)
	} else {
		detail = fmt.Sprintf("%s %s/%s failed. %s.", operation, r.spec.Kind, id, action)
	}
	if serverMsg == "" {
		return detail
	}
	return fmt.Sprintf("%s Gateway message: %s", detail, serverMsg)
}

// invalidAttribute returns the configurable schema attribute named as a whole
// word in a 400/422 message, if any. Computed-only attributes such as id and
// revision are ignored because the user cannot correct them. The gateway may
// spell the field in snake_case or camelCase, so both are matched.
func (r *kindResource) invalidAttribute(message string) string {
	best := ""
	for name, attribute := range r.spec.Schema.Attributes {
		if !attribute.IsRequired() && !attribute.IsOptional() {
			continue
		}
		if !containsWord(message, name) && !containsWord(message, camelCase(name)) {
			continue
		}
		if len(name) > len(best) || (len(name) == len(best) && name < best) {
			best = name
		}
	}
	return best
}

// containsWord reports whether message contains word delimited by non-word
// bytes. Underscores count as word bytes so attribute names match exactly
// rather than as a prefix or suffix of a longer identifier.
func containsWord(message, word string) bool {
	if word == "" {
		return false
	}
	for offset := 0; offset < len(message); {
		i := strings.Index(message[offset:], word)
		if i < 0 {
			return false
		}
		i += offset
		start, end := i, i+len(word)
		if (start == 0 || !isWordByte(message[start-1])) && (end == len(message) || !isWordByte(message[end])) {
			return true
		}
		offset = i + 1
	}
	return false
}

func isWordByte(b byte) bool {
	return b == '_' ||
		b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9'
}

// camelCase converts a snake_case attribute name to the spelling the gateway
// uses in error messages.
func camelCase(name string) string {
	if !strings.Contains(name, "_") {
		return name
	}
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}
