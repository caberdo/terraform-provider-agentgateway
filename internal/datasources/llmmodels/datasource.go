// Package llmmodels implements the agentgateway_llm_models data source.
package llmmodels

import (
	"bytes"
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

const kind = configresource.KindLLMModel

// entryObjectType must match the nested attributes of the models attribute.
var entryObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"id":           types.StringType,
	"name":         types.StringType,
	"display_name": types.StringType,
	"provider":     types.StringType,
	"model":        types.StringType,
	"visibility":   types.StringType,
}}

// DataSource lists the models the gateway serves.
type DataSource struct {
	client clientReader
}

// clientReader is the subset of client.Client this data source uses.
type clientReader interface {
	List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error)
}

// New returns the agentgateway_llm_models data source.
func New() datasource.DataSource { return &DataSource{} }

// Metadata sets the Terraform type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_llm_models"
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

// Schema returns the agentgateway_llm_models schema.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Models the gateway serves, sorted by id.",
		Attributes: map[string]datasourceschema.Attribute{
			"name_prefix": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include models whose id (usually the name) starts with this prefix.",
			},
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identity of the listing: `llm.model:<name_prefix>`.",
			},
			"models": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Models served by the gateway, sorted by id.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"id": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Stable identity of the model entry.",
						},
						"name": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Client-facing model name or wildcard match pattern.",
						},
						"display_name": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Label shown in the UI in place of `name`.",
						},
						"provider": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Provider name, or the custom provider object as JSON.",
						},
						"model": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Upstream model id.",
						},
						"visibility": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Whether clients may request the model directly (`public`) or only through a virtual model (`internal`).",
						},
					},
				},
			},
		},
	}
}

// readModel is the Terraform state shape of agentgateway_llm_models.
type readModel struct {
	NamePrefix types.String `tfsdk:"name_prefix"`
	ID         types.String `tfsdk:"id"`
	Models     types.List   `tfsdk:"models"`
}

// modelEntry is one element of the models list.
type modelEntry struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Provider    types.String `tfsdk:"provider"`
	Model       types.String `tfsdk:"model"`
	Visibility  types.String `tfsdk:"visibility"`
}

// modelValue is the part of an llm.model value this data source reads. Unknown
// fields are ignored by encoding/json.
type modelValue struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"displayName"`
	Visibility  string          `json:"visibility"`
	Provider    json.RawMessage `json:"provider"`
	Params      struct {
		Model string `json:"model"`
	} `json:"params"`
}

// Read lists the models and stores them sorted by id.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config readModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("agentgateway_llm_models read failed", "the provider client is not configured")
		return
	}

	prefix := ""
	if !config.NamePrefix.IsNull() && !config.NamePrefix.IsUnknown() {
		prefix = config.NamePrefix.ValueString()
	}

	resources, err := d.client.List(ctx, kind)
	if err != nil {
		resp.Diagnostics.AddError(
			"agentgateway_llm_models read failed",
			fmt.Sprintf("%s; verify the gateway is reachable and the endpoint serves the config API", err),
		)
		return
	}

	filtered := resourceutil.FilterAndSort(resources, prefix)
	entries := make([]modelEntry, 0, len(filtered))
	for _, resource := range filtered {
		var value modelValue
		if err := json.Unmarshal(resource.Value, &value); err != nil {
			resp.Diagnostics.AddError(
				"agentgateway_llm_models read failed",
				fmt.Sprintf("could not decode the value of %s/%s; the gateway returned a value this provider cannot read", resource.Kind, resource.ID),
			)
			return
		}

		name := value.Name
		if name == "" {
			name = resource.ID
		}
		visibility := value.Visibility
		if visibility == "" {
			// The resource defaults visibility to public, so a model stored
			// without the field reads back as public rather than empty.
			visibility = "public"
		}
		entries = append(entries, modelEntry{
			ID:          types.StringValue(resource.ID),
			Name:        types.StringValue(name),
			DisplayName: types.StringValue(value.DisplayName),
			Provider:    types.StringValue(providerString(value.Provider)),
			Model:       types.StringValue(value.Params.Model),
			Visibility:  types.StringValue(visibility),
		})
	}

	models, diags := types.ListValueFrom(ctx, entryObjectType, entries)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := readModel{
		NamePrefix: config.NamePrefix,
		ID:         types.StringValue(string(kind) + ":" + prefix),
		Models:     models,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// providerString renders the provider union as a string. A built-in provider is
// a JSON string; a custom provider is an object, which is returned as compact
// JSON so the listing stays informative without inventing a second attribute.
func providerString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return name
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return ""
	}
	return compact.String()
}
