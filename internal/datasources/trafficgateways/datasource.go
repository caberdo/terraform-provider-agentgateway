// Package trafficgateways implements the agentgateway_traffic_gateways data
// source (design §8): a read-only listing of the traffic.gateway config
// resources the gateway currently holds.
package trafficgateways

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// typeName is the Terraform type name of the data source.
	typeName = "agentgateway_traffic_gateways"
	// kind is the config resource kind the data source reads.
	kind = configresource.KindTrafficGateway
	// defaultTimeout matches the provider's default timeout (design §6).
	defaultTimeout = 30 * time.Second
)

// listenerModel is one named listener of a gateway.
type listenerModel struct {
	Name     types.String `tfsdk:"name"`
	Hostname types.String `tfsdk:"hostname"`
	Protocol types.String `tfsdk:"protocol"`
}

// gatewayModel is one gateway in the listing.
type gatewayModel struct {
	Name        types.String    `tfsdk:"name"`
	Port        types.Int64     `tfsdk:"port"`
	BindAddress types.String    `tfsdk:"bind_address"`
	Protocol    types.String    `tfsdk:"protocol"`
	Listeners   []listenerModel `tfsdk:"listeners"`
}

// model is the data source configuration and state.
type model struct {
	ID         types.String   `tfsdk:"id"`
	NamePrefix types.String   `tfsdk:"name_prefix"`
	Gateways   []gatewayModel `tfsdk:"gateways"`
}

// gatewayValue is the wire representation of a traffic.gateway value. Every
// field is a pointer so an absent field stays null instead of becoming zero.
type gatewayValue struct {
	Name        *string          `json:"name"`
	Port        *int64           `json:"port"`
	BindAddress *string          `json:"bindAddress"`
	Protocol    *string          `json:"protocol"`
	Listeners   *[]listenerValue `json:"listeners"`
}

// listenerValue is the wire representation of one gateway listener.
type listenerValue struct {
	Name     *string `json:"name"`
	Hostname *string `json:"hostname"`
	Protocol *string `json:"protocol"`
}

// dataSource is the agentgateway_traffic_gateways implementation.
type dataSource struct {
	data *providerdata.Data
}

// New returns the data source.
func New() datasource.DataSource {
	return &dataSource{}
}

// Metadata returns the Terraform type name.
func (d *dataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = typeName
}

// Schema returns the data source schema.
func (d *dataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the `traffic.gateway` config resources the gateway holds, sorted by name. " +
			"Use `name_prefix` to narrow the listing.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier of the listing: `traffic.gateway:<name_prefix>`.",
			},
			"name_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include gateways whose name starts with this prefix. An empty prefix includes every gateway.",
			},
			"gateways": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The matching gateways, sorted by name.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Gateway name, which is also the resource id.",
						},
						"port": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Port the gateway binds, when set at the gateway level.",
						},
						"bind_address": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Address the gateway binds, when set at the gateway level.",
						},
						"protocol": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Gateway-level protocol: `HTTP`, `HTTPS`, `TCP` or `TLS`.",
						},
						"listeners": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Named listeners of the gateway.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Listener name.",
									},
									"hostname": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Hostname the listener matches.",
									},
									"protocol": schema.StringAttribute{
										Computed:            true,
										MarkdownDescription: "Listener protocol.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure stores the provider data for later reads.
func (d *dataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			typeName+" configure failed",
			fmt.Sprintf("expected *providerdata.Data, got %T", req.ProviderData),
		)
		return
	}
	d.data = data
}

// Read lists the gateways and stores the filtered, sorted listing in state.
func (d *dataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	data := d.data
	if data == nil {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			"the provider was not configured; set the gateway endpoint in the provider block or AGENTGATEWAY_ENDPOINT and re-apply",
		)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(data.Timeout))
	defer cancel()

	var model model
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resources, err := data.Client.List(ctx, kind)
	if err != nil {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			fmt.Sprintf("the gateway could not list %s resources: %s. Check that the gateway is reachable and retry.", kind, err.Error()),
		)
		return
	}

	filtered := resourceutil.FilterAndSort(resources, model.NamePrefix.ValueString())
	gateways, diags := flatten(filtered)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	model.ID = types.StringValue(listID(model.NamePrefix.ValueString()))
	model.Gateways = gateways

	tflog.Debug(ctx, "listed traffic gateways", map[string]any{
		"kind":  kind,
		"count": len(gateways),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// listID returns the stable identifier of a listing: the kind plus the filter.
func listID(prefix string) string {
	return string(kind) + ":" + prefix
}

// flatten converts the filtered resources into the listing model.
func flatten(resources []configresource.Resource) ([]gatewayModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]gatewayModel, 0, len(resources))

	for _, resource := range resources {
		var value gatewayValue
		if err := json.Unmarshal(resource.Value, &value); err != nil {
			diags.AddError(
				typeName+" read failed",
				fmt.Sprintf("%s/%s could not be decoded as a traffic gateway; the gateway returned a value this provider version does not understand", kind, resource.ID),
			)
			continue
		}
		out = append(out, flattenGateway(resource.ID, value))
	}
	return out, diags
}

// flattenGateway converts one decoded gateway value into its model.
func flattenGateway(id string, value gatewayValue) gatewayModel {
	name := types.StringValue(id)
	if value.Name != nil {
		name = types.StringValue(*value.Name)
	}

	var listeners []listenerModel
	if value.Listeners != nil {
		listeners = make([]listenerModel, 0, len(*value.Listeners))
		for _, listener := range *value.Listeners {
			listeners = append(listeners, listenerModel{
				Name:     types.StringPointerValue(listener.Name),
				Hostname: types.StringPointerValue(listener.Hostname),
				Protocol: types.StringPointerValue(listener.Protocol),
			})
		}
	}

	return gatewayModel{
		Name:        name,
		Port:        types.Int64PointerValue(value.Port),
		BindAddress: types.StringPointerValue(value.BindAddress),
		Protocol:    types.StringPointerValue(value.Protocol),
		Listeners:   listeners,
	}
}

// timeout returns the effective read timeout.
func timeout(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultTimeout
	}
	return d
}
