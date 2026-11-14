package llmprovider

import (
	"context"
	"encoding/json"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// providerOptions are the three mutually exclusive ways to name a provider
// (spec C16): a built-in or preset string, a reference to another provider
// entry, or a custom provider object.
//
// The string option is exposed as provider_name, not provider: Terraform
// reserves `provider` as a root attribute name in resource blocks (it is the
// provider meta-argument), so the framework rejects a schema that uses it. The
// API value is still the `provider` field.
var providerOptions = []path.Expression{
	path.MatchRoot("provider_name"),
	path.MatchRoot("provider_reference"),
	path.MatchRoot("custom_provider_json"),
}

// newSchema returns the agentgateway_llm_provider schema.
func newSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "Reusable LLM provider defaults that models reference by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Provider entry name. Models reference it with `provider_reference`.",
			},
			"provider_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Built-in or preset provider name. Exactly one of `provider_name`, `provider_reference` or `custom_provider_json` must be set.",
				Validators: []validator.String{
					stringvalidator.OneOf(providerNames()...),
					stringvalidator.ExactlyOneOf(providerOptions...),
				},
			},
			"provider_reference": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of another provider entry to inherit from. Exactly one of `provider_name`, `provider_reference` or `custom_provider_json` must be set.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(providerOptions...),
				},
			},
			"custom_provider_json": schema.StringAttribute{
				Optional:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Custom provider as JSON, `{model, providerOverride, formats}`. Exactly one of `provider_name`, `provider_reference` or `custom_provider_json` must be set.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(providerOptions...),
				},
			},
			"model": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Upstream model id (`params.model`).",
			},
			"api_key": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "Upstream credential (`params.apiKey`). Supports `$ENV` references and `{file: …}`.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Upstream base URL (`params.baseUrl`).",
			},
			"host_override": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Upstream host override (`params.hostOverride`).",
			},
			"path_override": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Upstream path override (`params.pathOverride`).",
			},
			"path_prefix": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path prefix to strip before forwarding (`params.pathPrefix`).",
			},
			"aws_region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "AWS region for `bedrock` (`params.awsRegion`).",
			},
			"vertex_region": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Vertex AI region (`params.vertexRegion`).",
			},
			"vertex_project": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Vertex AI project (`params.vertexProject`).",
			},
			"azure_resource_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure resource name (`params.azureResourceName`).",
			},
			"azure_resource_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure resource type (`params.azureResourceType`).",
			},
			"azure_api_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure API version (`params.azureApiVersion`).",
			},
			"azure_project_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure project name (`params.azureProjectName`).",
			},
			"tokenize": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Ask the gateway to tokenize requests for this provider (`params.tokenize`).",
			},
			"defaults_json": schema.StringAttribute{
				Optional:            true,
				CustomType:          jsontypes.NormalizedType{},
				MarkdownDescription: "Provider defaults as JSON: `defaults`, `overrides`, `transformations`, TLS, auth and health.",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Provider name as stored by the gateway.",
			},
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the provider.",
			},
		},
	}
}

// providerNames returns the built-in and preset provider names. The list is
// owned by configresource and must never be retyped here.
func providerNames() []string {
	names := make([]string, 0, len(configresource.BuiltinProviders)+len(configresource.ProviderPresets))
	names = append(names, configresource.BuiltinProviders...)
	names = append(names, configresource.ProviderPresets...)
	return names
}

// llmProviderModel is the Terraform state shape of agentgateway_llm_provider.
type llmProviderModel struct {
	Name               types.String         `tfsdk:"name"`
	ProviderName       types.String         `tfsdk:"provider_name"`
	ProviderReference  types.String         `tfsdk:"provider_reference"`
	CustomProviderJSON jsontypes.Normalized `tfsdk:"custom_provider_json"`
	Model              types.String         `tfsdk:"model"`
	APIKey             types.String         `tfsdk:"api_key"`
	BaseURL            types.String         `tfsdk:"base_url"`
	HostOverride       types.String         `tfsdk:"host_override"`
	PathOverride       types.String         `tfsdk:"path_override"`
	PathPrefix         types.String         `tfsdk:"path_prefix"`
	AWSRegion          types.String         `tfsdk:"aws_region"`
	VertexRegion       types.String         `tfsdk:"vertex_region"`
	VertexProject      types.String         `tfsdk:"vertex_project"`
	AzureResourceName  types.String         `tfsdk:"azure_resource_name"`
	AzureResourceType  types.String         `tfsdk:"azure_resource_type"`
	AzureAPIVersion    types.String         `tfsdk:"azure_api_version"`
	AzureProjectName   types.String         `tfsdk:"azure_project_name"`
	Tokenize           types.Bool           `tfsdk:"tokenize"`
	DefaultsJSON       jsontypes.Normalized `tfsdk:"defaults_json"`
	IDValue            types.String         `tfsdk:"id"`
	Revision           types.Int64          `tfsdk:"revision"`
}

// Kind returns the config resource kind.
func (m *llmProviderModel) Kind() configresource.Kind { return configresource.KindLLMProvider }

// ID returns the provider name, which is the config resource id.
func (m *llmProviderModel) ID() string { return m.Name.ValueString() }

// Value builds the API value object from the model.
func (m *llmProviderModel) Value(ctx context.Context, diags *diag.Diagnostics) json.RawMessage {
	if m.Name.IsNull() || m.Name.IsUnknown() || m.Name.ValueString() == "" {
		diags.AddError("Invalid agentgateway_llm_provider", "name is required and must be a non-empty string")
		return nil
	}

	value := map[string]any{"name": m.Name.ValueString()}

	provider, ok := m.providerValue(ctx, diags)
	if !ok {
		return nil
	}
	if provider != nil {
		value["provider"] = provider
	}

	params := map[string]any{}
	putString(params, "model", m.Model)
	putString(params, "apiKey", m.APIKey)
	putString(params, "baseUrl", m.BaseURL)
	putString(params, "hostOverride", m.HostOverride)
	putString(params, "pathOverride", m.PathOverride)
	putString(params, "pathPrefix", m.PathPrefix)
	putString(params, "awsRegion", m.AWSRegion)
	putString(params, "vertexRegion", m.VertexRegion)
	putString(params, "vertexProject", m.VertexProject)
	putString(params, "azureResourceName", m.AzureResourceName)
	putString(params, "azureResourceType", m.AzureResourceType)
	putString(params, "azureApiVersion", m.AzureAPIVersion)
	putString(params, "azureProjectName", m.AzureProjectName)
	if !m.Tokenize.IsNull() && !m.Tokenize.IsUnknown() {
		params["tokenize"] = m.Tokenize.ValueBool()
	}
	if len(params) > 0 {
		value["params"] = params
	}

	if presentJSON(m.DefaultsJSON) {
		value["defaults"] = json.RawMessage(m.DefaultsJSON.ValueString())
	}

	raw, err := json.Marshal(value)
	if err != nil {
		diags.AddError("Invalid agentgateway_llm_provider", "the provider could not be encoded as JSON")
		return nil
	}
	return raw
}

// providerValue encodes the provider union (spec C16). Exactly one option is
// required; the schema validators catch the common case and this check is the
// belt-and-braces guard the harness relies on.
func (m *llmProviderModel) providerValue(_ context.Context, diags *diag.Diagnostics) (any, bool) {
	set := 0
	for _, value := range []bool{
		setString(m.ProviderName),
		setString(m.ProviderReference),
		setJSON(m.CustomProviderJSON),
	} {
		if value {
			set++
		}
	}

	switch set {
	case 1:
	case 0:
		diags.AddError(
			"Invalid agentgateway_llm_provider",
			"exactly one of provider_name, provider_reference or custom_provider_json must be set",
		)
		return nil, false
	default:
		diags.AddError(
			"Invalid agentgateway_llm_provider",
			"only one of provider_name, provider_reference or custom_provider_json may be set",
		)
		return nil, false
	}

	switch {
	case setString(m.ProviderName):
		return m.ProviderName.ValueString(), true
	case setString(m.ProviderReference):
		return map[string]any{"reference": m.ProviderReference.ValueString()}, true
	default:
		return map[string]any{"custom": json.RawMessage(m.CustomProviderJSON.ValueString())}, true
	}
}

// SetValue stores the provider returned by the gateway. Attributes the server
// did not return keep the value already in state.
func (m *llmProviderModel) SetValue(_ context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var body struct {
		Name     string                     `json:"name"`
		Provider json.RawMessage            `json:"provider"`
		Params   map[string]json.RawMessage `json:"params"`
		Defaults json.RawMessage            `json:"defaults"`
	}
	if err := json.Unmarshal(value, &body); err != nil {
		diags.AddError("Failed to read agentgateway_llm_provider", "the gateway returned a value that is not a JSON object")
		return
	}

	if body.Name != "" {
		m.Name = types.StringValue(body.Name)
	}

	m.setProvider(body.Provider, diags)
	m.setParams(body.Params)
	if isPresent(body.Defaults) {
		m.DefaultsJSON = jsontypes.NewNormalizedValue(string(body.Defaults))
	}
}

// setProvider decodes the provider union: a string, `{reference: name}` or
// `{custom: {...}}`. The three attributes are cleared as a group so state can
// never carry two options at once.
func (m *llmProviderModel) setProvider(raw json.RawMessage, diags *diag.Diagnostics) {
	if !isPresent(raw) {
		return
	}

	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		m.ProviderName = types.StringValue(name)
		m.ProviderReference = types.StringNull()
		m.CustomProviderJSON = jsontypes.NewNormalizedNull()
		return
	}

	var union map[string]json.RawMessage
	if err := json.Unmarshal(raw, &union); err != nil {
		diags.AddError("Failed to read agentgateway_llm_provider", "the gateway returned a provider value that is neither a name nor an object")
		return
	}

	if reference, ok := union["reference"]; ok {
		var referenceName string
		if err := json.Unmarshal(reference, &referenceName); err != nil {
			diags.AddError("Failed to read agentgateway_llm_provider", "the gateway returned a provider reference that is not a string")
			return
		}
		m.ProviderReference = types.StringValue(referenceName)
		m.ProviderName = types.StringNull()
		m.CustomProviderJSON = jsontypes.NewNormalizedNull()
		return
	}

	if custom, ok := union["custom"]; ok && isPresent(custom) {
		m.CustomProviderJSON = jsontypes.NewNormalizedValue(string(custom))
		m.ProviderName = types.StringNull()
		m.ProviderReference = types.StringNull()
		return
	}

	diags.AddError("Failed to read agentgateway_llm_provider", "the gateway returned a provider object that is neither a reference nor a custom provider")
}

// setParams stores the params the server returned. Missing keys are left as
// they are so an omitted attribute is never zeroed.
func (m *llmProviderModel) setParams(params map[string]json.RawMessage) {
	setParam(params, "model", &m.Model)
	setParam(params, "apiKey", &m.APIKey)
	setParam(params, "baseUrl", &m.BaseURL)
	setParam(params, "hostOverride", &m.HostOverride)
	setParam(params, "pathOverride", &m.PathOverride)
	setParam(params, "pathPrefix", &m.PathPrefix)
	setParam(params, "awsRegion", &m.AWSRegion)
	setParam(params, "vertexRegion", &m.VertexRegion)
	setParam(params, "vertexProject", &m.VertexProject)
	setParam(params, "azureResourceName", &m.AzureResourceName)
	setParam(params, "azureResourceType", &m.AzureResourceType)
	setParam(params, "azureApiVersion", &m.AzureAPIVersion)
	setParam(params, "azureProjectName", &m.AzureProjectName)

	if raw, ok := params["tokenize"]; ok && isPresent(raw) {
		var tokenize bool
		if err := json.Unmarshal(raw, &tokenize); err == nil {
			m.Tokenize = types.BoolValue(tokenize)
		}
	}
}

// SetServerMeta stores the computed id and revision.
func (m *llmProviderModel) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// setParam decodes one string parameter when the server returned it.
func setParam(params map[string]json.RawMessage, key string, target *types.String) {
	raw, ok := params[key]
	if !ok || !isPresent(raw) {
		return
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	*target = types.StringValue(value)
}

// putString adds a parameter when the Terraform value is set.
func putString(params map[string]any, key string, value types.String) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	params[key] = value.ValueString()
}

// isPresent reports whether raw carries a JSON value other than null.
func isPresent(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// setString reports whether a Terraform string is set.
func setString(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// setJSON reports whether a normalized JSON value is set.
func setJSON(value jsontypes.Normalized) bool {
	return !value.IsNull() && !value.IsUnknown()
}

// presentJSON reports whether a normalized JSON value should be sent.
func presentJSON(value jsontypes.Normalized) bool {
	return setJSON(value) && value.ValueString() != ""
}
