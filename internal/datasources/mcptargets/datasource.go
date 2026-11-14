// Package mcptargets implements the agentgateway_mcp_targets data source.
package mcptargets

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const kind = configresource.KindMCPTarget

// entryObjectType must match the nested attributes of the targets attribute.
var entryObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":      types.StringType,
	"transport": types.StringType,
	"host":      types.StringType,
	"path":      types.StringType,
}}

// DataSource lists the MCP targets the gateway stores.
type DataSource struct {
	client clientReader
}

// clientReader is the subset of client.Client this data source uses.
type clientReader interface {
	List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error)
}

// New returns the agentgateway_mcp_targets data source.
func New() datasource.DataSource { return &DataSource{} }

// Metadata sets the Terraform type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mcp_targets"
}

// Configure stores the provider client.
func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *providerdata.Data, got %T", req.ProviderData),
		)
		return
	}
	if data.Client == nil {
		resp.Diagnostics.AddError("Unexpected provider data", "the provider client is not configured")
		return
	}
	d.client = data.Client
}

// Schema returns the agentgateway_mcp_targets schema.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "MCP targets the gateway serves, sorted by id.",
		Attributes: map[string]datasourceschema.Attribute{
			"name_prefix": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include targets whose id (the name) starts with this prefix.",
			},
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identity of the listing: `mcp.target:<name_prefix>`.",
			},
			"targets": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "MCP targets stored by the gateway, sorted by id.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"name": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Target name, which prefixes tool names when multiplexing.",
						},
						"transport": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Transport of the target: `mcp`, `sse`, `stdio` or `openapi`.",
						},
						"host": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Host of the transport. Empty for `stdio` and for `mcp`, `sse` and `openapi` transports configured with `backend`.",
						},
						"path": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Path of the transport. Empty when the target does not set one.",
						},
					},
				},
			},
		},
	}
}

// readModel is the Terraform state shape of agentgateway_mcp_targets.
type readModel struct {
	NamePrefix types.String `tfsdk:"name_prefix"`
	ID         types.String `tfsdk:"id"`
	Targets    types.List   `tfsdk:"targets"`
}

// targetEntry is one element of the targets list.
type targetEntry struct {
	Name      types.String `tfsdk:"name"`
	Transport types.String `tfsdk:"transport"`
	Host      types.String `tfsdk:"host"`
	Path      types.String `tfsdk:"path"`
}

// networkValue is one of the HTTP-shaped transport blocks.
type networkValue struct {
	Host string `json:"host"`
	Path string `json:"path"`
}

// targetValue is the part of an mcp.target value this data source reads. A
// target has exactly one transport block (spec §7.7). Unknown fields are
// ignored by encoding/json.
type targetValue struct {
	Name    string        `json:"name"`
	MCP     *networkValue `json:"mcp"`
	SSE     *networkValue `json:"sse"`
	Stdio   *struct{}     `json:"stdio"`
	OpenAPI *networkValue `json:"openapi"`
}

// Read lists the MCP targets and stores them sorted by id.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config readModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("agentgateway_mcp_targets read failed", "the provider client is not configured")
		return
	}

	prefix := ""
	if !config.NamePrefix.IsNull() && !config.NamePrefix.IsUnknown() {
		prefix = config.NamePrefix.ValueString()
	}

	resources, err := d.client.List(ctx, kind)
	if err != nil {
		resp.Diagnostics.AddError(
			"agentgateway_mcp_targets read failed",
			fmt.Sprintf("%s; verify the gateway is reachable and the endpoint serves the config API", err),
		)
		return
	}

	filtered := resourceutil.FilterAndSort(resources, prefix)
	entries := make([]targetEntry, 0, len(filtered))
	for _, resource := range filtered {
		var value targetValue
		if err := json.Unmarshal(resource.Value, &value); err != nil {
			resp.Diagnostics.AddError(
				"agentgateway_mcp_targets read failed",
				fmt.Sprintf("could not decode the value of %s/%s; the gateway returned a value this provider cannot read", resource.Kind, resource.ID),
			)
			return
		}

		name := value.Name
		if name == "" {
			name = resource.ID
		}
		transport, host, path := transportOf(value)
		entries = append(entries, targetEntry{
			Name:      types.StringValue(name),
			Transport: types.StringValue(transport),
			Host:      types.StringValue(host),
			Path:      types.StringValue(path),
		})
	}

	targets, diags := types.ListValueFrom(ctx, entryObjectType, entries)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := readModel{
		NamePrefix: config.NamePrefix,
		ID:         types.StringValue(string(kind) + ":" + prefix),
		Targets:    targets,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// transportOf returns the transport name and its network address. The order
// matches the resource schema; the server enforces exactly one block.
func transportOf(value targetValue) (transport, host, path string) {
	switch {
	case value.MCP != nil:
		return "mcp", value.MCP.Host, value.MCP.Path
	case value.SSE != nil:
		return "sse", value.SSE.Host, value.SSE.Path
	case value.Stdio != nil:
		return "stdio", "", ""
	case value.OpenAPI != nil:
		return "openapi", value.OpenAPI.Host, value.OpenAPI.Path
	default:
		return "", "", ""
	}
}
