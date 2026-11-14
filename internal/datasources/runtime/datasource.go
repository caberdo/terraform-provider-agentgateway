// Package runtime implements the agentgateway_runtime data source.
package runtime

import (
	"context"
	"fmt"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DataSource reads the gateway runtime document.
type DataSource struct {
	client runtimeReader
}

// runtimeReader is the subset of client.Client this data source uses.
type runtimeReader interface {
	Runtime(ctx context.Context) (client.RuntimeInfo, error)
}

// New returns the agentgateway_runtime data source.
func New() datasource.DataSource { return &DataSource{} }

// Metadata sets the Terraform type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_runtime"
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

// Schema returns the agentgateway_runtime schema.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Build, mode and config-reload information reported by GET /api/runtime.",
		Attributes: map[string]datasourceschema.Attribute{
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identity of the data source: `runtime`.",
			},
			"build_version": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Gateway build version (`build.version`).",
			},
			"git_revision": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Git revision the gateway was built from.",
			},
			"gateway_mode": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Gateway mode, `standalone` or `xds`.",
			},
			"config_store_mode": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Config storage mode, `file`, `hybrid` or `readOnly`.",
			},
			"config_reload_synchronized": datasourceschema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the last config reload completed successfully.",
			},
			"config_reload_last_error": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Error from the last config reload, empty when it succeeded.",
			},
		},
	}
}

// readModel is the Terraform state shape of agentgateway_runtime.
type readModel struct {
	ID                       types.String `tfsdk:"id"`
	BuildVersion             types.String `tfsdk:"build_version"`
	GitRevision              types.String `tfsdk:"git_revision"`
	GatewayMode              types.String `tfsdk:"gateway_mode"`
	ConfigStoreMode          types.String `tfsdk:"config_store_mode"`
	ConfigReloadSynchronized types.Bool   `tfsdk:"config_reload_synchronized"`
	ConfigReloadLastError    types.String `tfsdk:"config_reload_last_error"`
}

// Read fetches the runtime document and stores it.
func (d *DataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("agentgateway_runtime read failed", "the provider client is not configured")
		return
	}

	info, err := d.client.Runtime(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"agentgateway_runtime read failed",
			fmt.Sprintf("%s; verify the gateway is reachable and the endpoint serves the admin API", err),
		)
		return
	}

	state := readModel{
		ID:                       types.StringValue("runtime"),
		BuildVersion:             types.StringValue(info.Version),
		GitRevision:              types.StringValue(info.GitRevision),
		GatewayMode:              types.StringValue(info.GatewayMode),
		ConfigStoreMode:          types.StringValue(info.ConfigStoreMode),
		ConfigReloadSynchronized: types.BoolValue(info.ConfigReloadOK),
		ConfigReloadLastError:    types.StringValue(info.ConfigReloadErr),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
