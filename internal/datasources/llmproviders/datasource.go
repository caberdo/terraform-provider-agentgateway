// Package llmproviders implements the agentgateway_llm_providers data source.
package llmproviders

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

const kind = configresource.KindLLMProvider

// entryObjectType must match the nested attributes of the providers attribute.
var entryObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":     types.StringType,
	"provider": types.StringType,
	"model":    types.StringType,
	"base_url": types.StringType,
}}

// DataSource lists the reusable provider entries the gateway stores.
type DataSource struct {
	client clientReader
}

// clientReader is the subset of client.Client this data source uses.
type clientReader interface {
	List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error)
}

// New returns the agentgateway_llm_providers data source.
func New() datasource.DataSource { return &DataSource{} }

// Metadata sets the Terraform type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_llm_providers"
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

// Schema returns the agentgateway_llm_providers schema.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Reusable LLM provider entries, sorted by id.",
		Attributes: map[string]datasourceschema.Attribute{
			"name_prefix": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include providers whose id (the name) starts with this prefix.",
			},
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identity of the listing: `llm.provider:<name_prefix>`.",
			},
			"providers": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Provider entries stored by the gateway, sorted by id.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"name": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Provider entry name.",
						},
						"provider": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Provider name, or the custom provider object as JSON.",
						},
						"model": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Upstream model id.",
						},
						"base_url": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Upstream base URL.",
						},
					},
				},
			},
		},
	}
}

// readModel is the Terraform state shape of agentgateway_llm_providers.
type readModel struct {
	NamePrefix types.String `tfsdk:"name_prefix"`
	ID         types.String `tfsdk:"id"`
	Providers  types.List   `tfsdk:"providers"`
}

// providerEntry is one element of the providers list.
type providerEntry struct {
	Name     types.String `tfsdk:"name"`
	Provider types.String `tfsdk:"provider"`
	Model    types.String `tfsdk:"model"`
	BaseURL  types.String `tfsdk:"base_url"`
}

// providerValue is the part of an llm.provider value this data source reads.
// Unknown fields are ignored by encoding/json.
type providerValue struct {
	Name     string          `json:"name"`
	Provider json.RawMessage `json:"provider"`
	Params   struct {
		Model   string `json:"model"`
		BaseURL string `json:"baseUrl"`
	} `json:"params"`
}

// Read lists the provider entries and stores them sorted by id.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config readModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("agentgateway_llm_providers read failed", "the provider client is not configured")
		return
	}

	prefix := ""
	if !config.NamePrefix.IsNull() && !config.NamePrefix.IsUnknown() {
		prefix = config.NamePrefix.ValueString()
	}

	resources, err := d.client.List(ctx, kind)
	if err != nil {
		resp.Diagnostics.AddError(
			"agentgateway_llm_providers read failed",
			fmt.Sprintf("%s; verify the gateway is reachable and the endpoint serves the config API", err),
		)
		return
	}

	filtered := resourceutil.FilterAndSort(resources, prefix)
	entries := make([]providerEntry, 0, len(filtered))
	for _, resource := range filtered {
		var value providerValue
		if err := json.Unmarshal(resource.Value, &value); err != nil {
			resp.Diagnostics.AddError(
				"agentgateway_llm_providers read failed",
				fmt.Sprintf("could not decode the value of %s/%s; the gateway returned a value this provider cannot read", resource.Kind, resource.ID),
			)
			return
		}

		name := value.Name
		if name == "" {
			name = resource.ID
		}
		entries = append(entries, providerEntry{
			Name:     types.StringValue(name),
			Provider: types.StringValue(providerString(value.Provider)),
			Model:    types.StringValue(value.Params.Model),
			BaseURL:  types.StringValue(value.Params.BaseURL),
		})
	}

	providers, diags := types.ListValueFrom(ctx, entryObjectType, entries)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := readModel{
		NamePrefix: config.NamePrefix,
		ID:         types.StringValue(string(kind) + ":" + prefix),
		Providers:  providers,
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
