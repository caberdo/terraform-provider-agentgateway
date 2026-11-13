package llmsettings

import (
	"context"
	"fmt"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// New returns the agentgateway_llm_settings resource.
func New() resource.Resource {
	return &settingsResource{inner: resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_llm_settings",
		Kind:     configresource.KindLLMSettings,
		Schema:   newSchema(),
		// The list is seeded with its element type: ImportState writes the fresh
		// model straight into state, and a zero-value types.List carries no
		// element type, which the framework then rejects on the next Read.
		New: func() resourceutil.KindResource {
			return &llmSettingsModel{Gateways: types.ListNull(types.StringType)}
		},
	})}
}

// settingsResource decorates the shared CRUD harness with the settings-specific
// 409 mapping. The harness is still the only writer: this type delegates every
// operation and only rewrites the diagnostic a file-owned conflict produces.
type settingsResource struct {
	inner resource.Resource
}

func (r *settingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	r.inner.Metadata(ctx, req, resp)
}

func (r *settingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	r.inner.Schema(ctx, req, resp)
}

func (r *settingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configured, ok := r.inner.(resource.ResourceWithConfigure)
	if !ok {
		resp.Diagnostics.AddError(
			"agentgateway_llm_settings is not configurable",
			"the shared CRUD harness does not implement Configure; this is a provider bug",
		)
		return
	}
	configured.Configure(ctx, req, resp)
}

func (r *settingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.inner.Create(ctx, req, resp)
	rewriteFileOwned(&resp.Diagnostics)
}

func (r *settingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.inner.Read(ctx, req, resp)
}

func (r *settingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.inner.Update(ctx, req, resp)
	rewriteFileOwned(&resp.Diagnostics)
}

func (r *settingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.inner.Delete(ctx, req, resp)
}

func (r *settingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importer, ok := r.inner.(resource.ResourceWithImportState)
	if !ok {
		resp.Diagnostics.AddError(
			"agentgateway_llm_settings cannot be imported",
			"the shared CRUD harness does not implement ImportState; this is a provider bug",
		)
		return
	}
	importer.ImportState(ctx, req, resp)
}

// rewriteFileOwned replaces the harness's generic conflict diagnostic with the
// settings-specific one when the gateway reports a field the config file owns
// (spec C9). Every other diagnostic is left untouched.
func rewriteFileOwned(diags *diag.Diagnostics) {
	if diags == nil || len(*diags) == 0 {
		return
	}

	rewritten := make(diag.Diagnostics, 0, len(*diags))
	for _, diagnostic := range *diags {
		if diagnostic.Severity() != diag.SeverityError || !strings.Contains(diagnostic.Detail(), "file-owned") {
			rewritten = append(rewritten, diagnostic)
			continue
		}
		rewritten.AddError(diagnostic.Summary(), fileOwnedDetail(diagnostic.Detail()))
	}
	*diags = rewritten
}

// fileOwnedDetail explains that the config file owns the field and points at
// the setting that decides who is allowed to write it.
func fileOwnedDetail(serverDetail string) string {
	field := ownedField(serverDetail)
	if field == "" {
		return "The gateway's config file owns this field, so Terraform cannot change it while config.storage.mode = hybrid. Remove the field from the config file and restart the gateway to manage it with Terraform, or keep managing it in the file."
	}
	return fmt.Sprintf(
		"The gateway's config file owns the %s field, so Terraform cannot change it while config.storage.mode = hybrid. Remove %s from the config file and restart the gateway to manage it with Terraform, or keep managing it in the file.",
		field, field,
	)
}

// ownedField extracts the field name from the gateway's message, for example
// "… field cannot be updated in hybrid mode: gateways". It returns "" when the
// message does not name a field.
func ownedField(serverDetail string) string {
	index := strings.LastIndex(serverDetail, ": ")
	if index < 0 {
		return ""
	}
	field := strings.TrimSpace(serverDetail[index+2:])
	field = strings.TrimSuffix(field, ".")
	if field == "" || strings.ContainsAny(field, " \t\n\"'") {
		return ""
	}
	return field
}
