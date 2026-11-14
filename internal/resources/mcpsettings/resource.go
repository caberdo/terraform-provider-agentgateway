package mcpsettings

import (
	"context"
	"fmt"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// New returns the agentgateway_mcp_settings resource.
func New() resource.Resource {
	inner := resourceutil.CRUD(resourceutil.Spec{
		TypeName: "agentgateway_mcp_settings",
		Kind:     configresource.KindMCPSettings,
		Schema:   resourceSchema(),
		// The list is seeded with its element type: ImportState writes the fresh
		// model straight into state, and a zero-value types.List carries no
		// element type, which the framework then rejects on the next Read.
		New: func() resourceutil.KindResource {
			return &model{Gateways: types.ListNull(types.StringType)}
		},
	})
	return &fileOwnedResource{Resource: inner}
}

// fileOwnedResource refines the shared harness's 409 diagnostic. The harness
// maps every conflict to "conflict detected", but for settings the gateway
// says "file-owned ... field cannot be updated in hybrid mode" and the user
// needs to know that the config file, not Terraform, owns the field (spec C9).
type fileOwnedResource struct {
	resource.Resource
}

var (
	_ resource.ResourceWithConfigure   = (*fileOwnedResource)(nil)
	_ resource.ResourceWithImportState = (*fileOwnedResource)(nil)
)

func (r *fileOwnedResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if configured, ok := r.Resource.(resource.ResourceWithConfigure); ok {
		configured.Configure(ctx, req, resp)
	}
}

func (r *fileOwnedResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.Resource.Create(ctx, req, resp)
	resp.Diagnostics = rewriteFileOwned(resp.Diagnostics)
}

func (r *fileOwnedResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.Resource.Update(ctx, req, resp)
	resp.Diagnostics = rewriteFileOwned(resp.Diagnostics)
}

func (r *fileOwnedResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importer, ok := r.Resource.(resource.ResourceWithImportState)
	if !ok {
		resp.Diagnostics.AddError(
			"agentgateway_mcp_settings import failed",
			"the shared resource harness does not support import",
		)
		return
	}
	importer.ImportState(ctx, req, resp)
}

const fileOwnedExplanation = "The gateway's config file owns this field, so Terraform cannot change it while config.storage.mode = hybrid. " +
	"Remove the field from the gateway config file and restart the gateway to manage it with Terraform, or keep managing it in the file."

// rewriteFileOwned replaces the harness's generic conflict detail for a
// file-owned settings field with one that names the real owner. Other
// diagnostics pass through untouched.
func rewriteFileOwned(diags diag.Diagnostics) diag.Diagnostics {
	rewritten := false
	for i, d := range diags {
		if d.Severity() != diag.SeverityError || !strings.Contains(d.Detail(), "file-owned") {
			continue
		}
		if !rewritten {
			out := make(diag.Diagnostics, len(diags))
			copy(out, diags)
			diags = out
			rewritten = true
		}
		diags[i] = diag.NewErrorDiagnostic(d.Summary(), fileOwnedDetail(d.Detail()))
	}
	return diags
}

// fileOwnedDetail explains that the config file owns the field, naming the
// field the gateway reported when its message carries one.
func fileOwnedDetail(detail string) string {
	field := ownedField(detail)
	if field == "" {
		return fileOwnedExplanation + " Gateway message: " + gatewayMessage(detail)
	}
	return fmt.Sprintf(
		"The gateway's config file owns the %s field, so Terraform cannot change it while config.storage.mode = hybrid. "+
			"Remove %s from the gateway config file and restart the gateway to manage it with Terraform, or keep managing it in the file. "+
			"Gateway message: %s",
		field, field, gatewayMessage(detail),
	)
}

// ownedField extracts the field name the gateway named after the final colon,
// for example "… field cannot be updated in hybrid mode: statefulMode". It
// returns "" when the message does not name a field.
func ownedField(detail string) string {
	index := strings.LastIndex(detail, ": ")
	if index < 0 {
		return ""
	}
	field := strings.TrimSpace(detail[index+2:])
	field = strings.TrimSuffix(field, ".")
	if field == "" || strings.ContainsAny(field, " \t\n\"'") {
		return ""
	}
	return field
}

// gatewayMessage returns the server message part of a harness diagnostic.
func gatewayMessage(detail string) string {
	const marker = "Gateway message: "
	if i := strings.Index(detail, marker); i >= 0 {
		return detail[i+len(marker):]
	}
	return detail
}

// resourceSchema is the Terraform schema for the mcp.settings singleton.
func resourceSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Manages the agentgateway MCP settings singleton (`mcp.settings`, id `default`). " +
			"Fields the gateway config file owns are rejected with a diagnostic; the gateway must use `config.storage.mode = hybrid`.",
		Attributes: map[string]schema.Attribute{
			"gateways": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				// gateways is deliberately not Computed: an empty list and an
				// unset list mean the same thing upstream (attach to the
				// default gateway), so the read path treats a server-returned
				// `[]` as absent when the configuration leaves the attribute
				// unset. Computed would make the attribute sticky, so removing
				// it from the configuration would never detach the routes.
				MarkdownDescription: "Gateways that serve the MCP routes, as `<gateway>` or `<gateway>/<listener>`. Defaults to the gateway named `default` server-side. A non-empty list conflicts with `port`; an empty list does not.",
			},
			"port": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Port that serves the MCP routes. Deprecated upstream; use `gateways` instead. Conflicts with a non-empty `gateways` list.",
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"stateful_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether to keep a persistent MCP session across requests (`stateful`) or create one per request (`stateless`). " +
					"Defaults to `stateless` server-side. A value the gateway returns is stored; removing the attribute from the configuration keeps the current value.",
				Validators: []validator.String{
					stringvalidator.OneOf("stateless", "stateful"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"prefix_mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "How to namespace tool names when multiplexing: `always` prefixes with the target name, `conditional` only when there is more than one target, `never` leaves names unprefixed (they must then be unique). Defaults to `conditional` server-side.",
				Validators: []validator.String{
					stringvalidator.OneOf("always", "conditional", "never"),
				},
			},
			"failure_mode": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Behaviour when a target fails to initialize or fails during fanout: `failClosed` or `failOpen`.",
				Validators: []validator.String{
					stringvalidator.OneOf("failClosed", "failOpen"),
				},
			},
			"sse_keep_alive": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Interval at which SSE keep-alive comments are sent on long-lived MCP streams, as a positive Go duration such as `30s` or `1m30s`. Disabled when unset.",
				Validators: []validator.String{
					goDuration{},
				},
			},
			"dns_rebinding_protection": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "When true, MCP requests are rejected unless the Host and Origin headers are localhost. " +
					"Off by default server-side; a value the gateway returns is stored, and removing the attribute keeps the current value.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"server": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Overrides for the MCP `serverInfo` and gateway instructions reported to clients when multiplexing multiple targets.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Overrides `serverInfo.name`. Must be set together with `version`.",
						Validators: []validator.String{
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("version")),
						},
					},
					"version": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Overrides `serverInfo.version`. Must be set together with `name`.",
						Validators: []validator.String{
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("name")),
						},
					},
					"title": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Overrides `serverInfo.title`.",
					},
					"instructions": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Overrides the gateway preamble prepended to merged upstream instructions.",
					},
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Settings id, always `default`.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the settings overlay.",
			},
		},
	}
}
