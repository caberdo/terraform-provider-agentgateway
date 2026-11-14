// Package trafficroutes implements the agentgateway_traffic_routes data source
// (design §8): a read-only listing of the traffic.route config resources the
// gateway currently holds.
package trafficroutes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// typeName is the Terraform type name of the data source.
	typeName = "agentgateway_traffic_routes"
	// kind is the config resource kind the data source reads.
	kind = configresource.KindTrafficRoute
	// defaultTimeout matches the provider's default timeout (design §6).
	defaultTimeout = 30 * time.Second
)

// routeModel is one route in the listing.
type routeModel struct {
	Name      types.String           `tfsdk:"name"`
	Gateways  []types.String         `tfsdk:"gateways"`
	Hostnames []types.String         `tfsdk:"hostnames"`
	Backends  []jsontypes.Normalized `tfsdk:"backends"`
}

// model is the data source configuration and state.
type model struct {
	ID         types.String `tfsdk:"id"`
	NamePrefix types.String `tfsdk:"name_prefix"`
	Routes     []routeModel `tfsdk:"routes"`
}

// routeValue is the wire representation of a traffic.route value. Every field
// is a pointer so an absent field stays null instead of becoming zero.
type routeValue struct {
	Name      *string            `json:"name"`
	Gateways  *[]string          `json:"gateways"`
	Hostnames *[]string          `json:"hostnames"`
	Backends  *[]json.RawMessage `json:"backends"`
}

// dataSource is the agentgateway_traffic_routes implementation.
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
		MarkdownDescription: "Lists the `traffic.route` config resources the gateway holds, sorted by id. " +
			"Use `name_prefix` to narrow the listing.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier of the listing: `traffic.route:<name_prefix>`.",
			},
			"name_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include routes whose name starts with this prefix. An empty prefix includes every route.",
			},
			"routes": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The matching routes, sorted by id.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Route name, which is also the resource id.",
						},
						"gateways": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Gateways the route is attached to.",
						},
						"hostnames": schema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Hostnames the route matches.",
						},
						"backends": schema.ListAttribute{
							Computed:    true,
							ElementType: jsontypes.NormalizedType{},
							// Backends expose their raw `policies`, which carry
							// TLS material and authorization rules, so the
							// whole list is marked sensitive.
							Sensitive:           true,
							MarkdownDescription: "Backends of the route, each as its raw JSON object. Use `jsondecode` to inspect one.",
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

// Read lists the routes and stores the filtered, sorted listing in state.
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
	routes, diags := flatten(filtered)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	model.ID = types.StringValue(listID(model.NamePrefix.ValueString()))
	model.Routes = routes

	tflog.Debug(ctx, "listed traffic routes", map[string]any{
		"kind":  kind,
		"count": len(routes),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// listID returns the stable identifier of a listing: the kind plus the filter.
func listID(prefix string) string {
	return string(kind) + ":" + prefix
}

// flatten converts the filtered resources into the listing model.
func flatten(resources []configresource.Resource) ([]routeModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]routeModel, 0, len(resources))

	for _, resource := range resources {
		var value routeValue
		if err := json.Unmarshal(resource.Value, &value); err != nil {
			diags.AddError(
				typeName+" read failed",
				fmt.Sprintf("%s/%s could not be decoded as a traffic route; the gateway returned a value this provider version does not understand", kind, resource.ID),
			)
			continue
		}
		out = append(out, flattenRoute(resource.ID, value))
	}
	return out, diags
}

// flattenRoute converts one decoded route value into its model.
func flattenRoute(id string, value routeValue) routeModel {
	name := types.StringValue(id)
	if value.Name != nil {
		name = types.StringValue(*value.Name)
	}

	var backends []jsontypes.Normalized
	if value.Backends != nil {
		backends = make([]jsontypes.Normalized, 0, len(*value.Backends))
		for _, backend := range *value.Backends {
			backends = append(backends, jsontypes.NewNormalizedValue(string(backend)))
		}
	} else {
		// An omitted array is an empty collection, never null: null cannot be
		// iterated in Terraform.
		backends = []jsontypes.Normalized{}
	}

	return routeModel{
		Name:      name,
		Gateways:  stringList(value.Gateways),
		Hostnames: stringList(value.Hostnames),
		Backends:  backends,
	}
}

// stringList converts an optional JSON string array into a Terraform list. An
// omitted array is an empty collection, never null: null cannot be iterated in
// Terraform.
func stringList(values *[]string) []types.String {
	if values == nil {
		return []types.String{}
	}
	out := make([]types.String, 0, len(*values))
	for _, value := range *values {
		out = append(out, types.StringValue(value))
	}

	return out
}

// timeout returns the effective read timeout.
func timeout(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultTimeout
	}
	return d
}
