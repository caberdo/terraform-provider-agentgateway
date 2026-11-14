// Package configresources implements the agentgateway_config_resources data
// source (design §8): the deliberate escape hatch that lists any config
// resource kind, including kinds a future gateway release adds.
package configresources

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// typeName is the Terraform type name of the data source.
	typeName = "agentgateway_config_resources"
	// defaultTimeout matches the provider's default timeout (design §6).
	defaultTimeout = 30 * time.Second
)

// kindPattern validates a config resource kind loosely on purpose: the data
// source must accept a kind added by a future gateway release without a
// provider release, so it is not an enum.
var kindPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.]*$`)

// resourceModel is one resource in the listing.
type resourceModel struct {
	ID        types.String         `tfsdk:"id"`
	Value     jsontypes.Normalized `tfsdk:"value_json"`
	Revision  types.Int64          `tfsdk:"revision"`
	CreatedAt types.String         `tfsdk:"created_at"`
	UpdatedAt types.String         `tfsdk:"updated_at"`
}

// model is the data source configuration and state.
type model struct {
	Kind      types.String    `tfsdk:"kind"`
	IDPrefix  types.String    `tfsdk:"id_prefix"`
	ID        types.String    `tfsdk:"id"`
	Resources []resourceModel `tfsdk:"resources"`
}

// dataSource is the agentgateway_config_resources implementation.
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
		MarkdownDescription: "Lists the resources of any config resource kind, sorted by id. " +
			"`kind` is validated loosely so a kind added by a future gateway release works without a provider release. " +
			"`value_json` is marked sensitive because some kinds carry key material.",
		Attributes: map[string]schema.Attribute{
			"kind": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Config resource kind to list, such as `llm.provider`, `traffic.gateway` or `llm.apiKey`.",
				Validators: []validator.String{
					kindValidator(),
				},
			},
			"id_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include resources whose id starts with this prefix. An empty prefix includes every resource of the kind.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identifier of the listing: `<kind>:<id_prefix>`.",
			},
			"resources": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The matching resources, sorted by id.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Resource id.",
						},
						"value_json": schema.StringAttribute{
							Computed:            true,
							Sensitive:           true,
							CustomType:          jsontypes.NormalizedType{},
							MarkdownDescription: "The resource value exactly as the gateway stores it. Marked sensitive as a whole.",
						},
						"revision": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "Stored revision; `0` when the gateway omits it.",
						},
						"created_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Creation timestamp in RFC 3339 format, when the gateway reports it.",
						},
						"updated_at": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Last update timestamp in RFC 3339 format, when the gateway reports it.",
						},
					},
				},
			},
		},
	}
}

// kindValidator returns the loose kind validator used by the schema.
func kindValidator() validator.String {
	return stringvalidator.RegexMatches(
		kindPattern,
		"must start with a letter and contain only letters, digits and dots, such as llm.provider or traffic.gateway",
	)
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

// Read lists the requested kind and stores the filtered, sorted listing in
// state.
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

	kind := configresource.Kind(model.Kind.ValueString())
	if !kindPattern.MatchString(string(kind)) {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			fmt.Sprintf("%q is not a valid config resource kind; it must start with a letter and contain only letters, digits and dots", kind),
		)
		return
	}

	resources, err := data.Client.List(ctx, kind)
	if err != nil {
		resp.Diagnostics.AddError(
			typeName+" read failed",
			fmt.Sprintf("the gateway could not list %s resources: %s. Check that the kind exists on this gateway release and retry.", kind, err.Error()),
		)
		return
	}

	filtered := resourceutil.FilterAndSort(resources, model.IDPrefix.ValueString())
	model.ID = types.StringValue(listID(kind, model.IDPrefix.ValueString()))
	model.Resources = flatten(filtered)

	tflog.Debug(ctx, "listed config resources", map[string]any{
		"kind":  kind,
		"count": len(filtered),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// listID returns the stable identifier of a listing: the kind plus the filter.
func listID(kind configresource.Kind, prefix string) string {
	return string(kind) + ":" + prefix
}

// flatten converts the filtered resources into the listing model.
func flatten(resources []configresource.Resource) []resourceModel {
	out := make([]resourceModel, 0, len(resources))
	for _, resource := range resources {
		out = append(out, resourceModel{
			ID:        types.StringValue(resource.ID),
			Value:     valueJSON(resource.Value),
			Revision:  types.Int64Value(resource.Revision),
			CreatedAt: timestamp(resource.CreatedAt),
			UpdatedAt: timestamp(resource.UpdatedAt),
		})
	}
	return out
}

// valueJSON returns the stored value as a normalized JSON string, or null when
// the gateway stored no value.
func valueJSON(value []byte) jsontypes.Normalized {
	if len(value) == 0 {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(value))

}

// timestamp formats a stored timestamp, keeping it null when the gateway did
// not report one.
func timestamp(value time.Time) types.String {
	if value.IsZero() {
		return types.StringNull()
	}
	return types.StringValue(value.UTC().Format(time.RFC3339))
}

// timeout returns the effective read timeout.
func timeout(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultTimeout
	}
	return d
}
