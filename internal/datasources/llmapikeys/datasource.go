// Package llmapikeys implements the agentgateway_llm_api_keys data source.
//
// The data source lists API-key metadata only. Key material is never read into
// Terraform state: the schema has no key or key_hash attribute, and the value
// decoder ignores both fields (design §8 and §12).
package llmapikeys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const kind = configresource.KindLLMAPIKey

// entryObjectType must match the nested attributes of the keys attribute.
var entryObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"id":             types.StringType,
	"name":           types.StringType,
	"user":           types.StringType,
	"group":          types.StringType,
	"allowed_models": types.ListType{ElemType: types.StringType},
	"created_at":     types.StringType,
}}

// DataSource lists virtual API keys without exposing key material.
type DataSource struct {
	client clientReader
}

// clientReader is the subset of client.Client this data source uses.
type clientReader interface {
	List(ctx context.Context, kind configresource.Kind) ([]configresource.Resource, error)
}

// New returns the agentgateway_llm_api_keys data source.
func New() datasource.DataSource { return &DataSource{} }

// Metadata sets the Terraform type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_llm_api_keys"
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

// Schema returns the agentgateway_llm_api_keys schema. It deliberately has no
// key or key_hash attribute.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		MarkdownDescription: "Virtual API key metadata, sorted by id. Key material is never exposed.",
		Attributes: map[string]datasourceschema.Attribute{
			"name_prefix": datasourceschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only include keys whose id (a server-generated UUID) starts with this prefix.",
			},
			"id": datasourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Stable identity of the listing: `llm.apiKey:<name_prefix>`.",
			},
			"keys": datasourceschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Virtual API key metadata stored by the gateway, sorted by id. Key material is never returned.",
				NestedObject: datasourceschema.NestedAttributeObject{
					Attributes: map[string]datasourceschema.Attribute{
						"id": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Server-generated key id.",
						},
						"name": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Key name from `metadata.name`.",
						},
						"user": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Owning user from `metadata.user`.",
						},
						"group": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Owning group from `metadata.group`.",
						},
						"allowed_models": datasourceschema.ListAttribute{
							Computed:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Model patterns the key may request. Null means unconstrained; an empty list denies every model.",
						},
						"created_at": datasourceschema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Creation time in RFC3339 format.",
						},
					},
				},
			},
		},
	}
}

// readModel is the Terraform state shape of agentgateway_llm_api_keys.
type readModel struct {
	NamePrefix types.String `tfsdk:"name_prefix"`
	ID         types.String `tfsdk:"id"`
	Keys       types.List   `tfsdk:"keys"`
}

// keyEntry is one element of the keys list. It has no field for key material.
type keyEntry struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	User          types.String `tfsdk:"user"`
	Group         types.String `tfsdk:"group"`
	AllowedModels types.List   `tfsdk:"allowed_models"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

// apiKeyValue is the part of an llm.apiKey value this data source reads. The
// key and keyHash fields are deliberately absent: they are never decoded and
// never reach state or diagnostics.
type apiKeyValue struct {
	Metadata      map[string]json.RawMessage `json:"metadata"`
	AllowedModels *[]string                  `json:"allowedModels"`
}

// Read lists the API keys and stores their metadata sorted by id.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config readModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("agentgateway_llm_api_keys read failed", "the provider client is not configured")
		return
	}

	prefix := ""
	if !config.NamePrefix.IsNull() && !config.NamePrefix.IsUnknown() {
		prefix = config.NamePrefix.ValueString()
	}

	resources, err := d.client.List(ctx, kind)
	if err != nil {
		resp.Diagnostics.AddError(
			"agentgateway_llm_api_keys read failed",
			fmt.Sprintf("%s; verify the gateway is reachable and the endpoint serves the config API", err),
		)
		return
	}

	filtered := resourceutil.FilterAndSort(resources, prefix)
	entries := make([]keyEntry, 0, len(filtered))
	for _, resource := range filtered {
		var value apiKeyValue
		if err := json.Unmarshal(resource.Value, &value); err != nil {
			resp.Diagnostics.AddError(
				"agentgateway_llm_api_keys read failed",
				fmt.Sprintf("could not decode the value of %s/%s; the gateway returned a value this provider cannot read", resource.Kind, resource.ID),
			)
			return
		}

		allowedModels, diags := allowedModelsValue(ctx, value.AllowedModels)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		entries = append(entries, keyEntry{
			ID:            types.StringValue(keyID(resource, value.Metadata)),
			Name:          types.StringValue(metadataString(value.Metadata, "name")),
			User:          types.StringValue(metadataString(value.Metadata, "user")),
			Group:         types.StringValue(metadataString(value.Metadata, "group")),
			AllowedModels: allowedModels,
			CreatedAt:     createdAtValue(resource, value.Metadata),
		})
	}

	keys, diags := types.ListValueFrom(ctx, entryObjectType, entries)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := readModel{
		NamePrefix: config.NamePrefix,
		ID:         types.StringValue(string(kind) + ":" + prefix),
		Keys:       keys,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// keyID prefers the id the server copied into metadata; the envelope id is the
// fallback for servers that do not inject it.
func keyID(resource configresource.Resource, metadata map[string]json.RawMessage) string {
	if id := metadataString(metadata, "id"); id != "" {
		return id
	}
	return resource.ID
}

// createdAtValue prefers the envelope timestamp and falls back to the
// server-managed metadata key. It returns null when neither is available.
func createdAtValue(resource configresource.Resource, metadata map[string]json.RawMessage) types.String {
	if !resource.CreatedAt.IsZero() {
		return types.StringValue(resource.CreatedAt.UTC().Format(time.RFC3339))
	}
	if raw := metadataString(metadata, "agentgateway.dev/createdAt"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return types.StringValue(parsed.UTC().Format(time.RFC3339))
		}
		return types.StringValue(raw)
	}
	return types.StringNull()
}

// allowedModelsValue preserves the null/empty distinction from spec C13.
func allowedModelsValue(ctx context.Context, models *[]string) (types.List, diag.Diagnostics) {
	if models == nil {
		return types.ListNull(types.StringType), nil
	}
	return types.ListValueFrom(ctx, types.StringType, *models)
}

// metadataString decodes one metadata value as a string. Non-string values are
// ignored rather than failing the whole listing.
func metadataString(metadata map[string]json.RawMessage, key string) string {
	raw, ok := metadata[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}
