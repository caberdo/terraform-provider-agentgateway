package llmmodel

import (
	"context"
	"encoding/json"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// providerOptions are the three mutually exclusive ways to name a provider
// (spec C16): a built-in or preset string, a reference to another provider
// entry, or a custom provider object. The Terraform attribute is provider_name
// because Terraform reserves the root name provider; the payload key stays
// provider.
var providerOptions = []path.Expression{
	path.MatchRoot("provider_name"),
	path.MatchRoot("provider_reference"),
	path.MatchRoot("custom_provider_json"),
}

// visibilityValues are the only visibility values the gateway accepts.
var visibilityValues = []string{"public", "internal"}

// newSchema returns the agentgateway_llm_model schema.
func newSchema() schema.Schema {
	return schema.Schema{
		MarkdownDescription: "A model that clients request through agentgateway and how it maps to a provider.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Client-facing model name or wildcard match pattern.",
			},
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Stable identity of the model entry. Defaults to `name`; changing it renames the model in place.",
			},
			"display_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Terraform-side label only; agentgateway v1.7.0 has no `displayName` for models, so this value stays in state and is not sent to the gateway.",
			},
			"visibility": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("public"),
				MarkdownDescription: "Whether clients may request the model directly (`public`) or only through a virtual model (`internal`).",
				Validators: []validator.String{
					stringvalidator.OneOf(visibilityValues...),
				},
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
				MarkdownDescription: "Name of an `agentgateway_llm_provider` entry to inherit from. Exactly one of `provider_name`, `provider_reference` or `custom_provider_json` must be set.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(providerOptions...),
				},
			},
			"custom_provider_json": jsonAttributeWithValidators(
				"Custom provider as JSON, `{model, providerOverride, formats}`. Exactly one of `provider_name`, `provider_reference` or `custom_provider_json` must be set.",
				stringvalidator.ExactlyOneOf(providerOptions...),
			),
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
				MarkdownDescription: "Azure resource type, `openAI` or `foundry` (`params.azureResourceType`).",
			},
			"azure_api_version": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure API version (`params.azureApiVersion`).",
			},
			"azure_project_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Azure Foundry project name (`params.azureProjectName`).",
			},
			"tokenize": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Ask the gateway to tokenize requests for this model (`params.tokenize`).",
			},
			"passthrough_json":            jsonAttribute("Passthrough mode and detection settings (`passthrough`)."),
			"authorization_json":          jsonAttribute("Per-model HTTP authorization rules (`authorization`)."),
			"matches_json":                jsonAttribute("Extra request conditions for selecting this model (`matches`)."),
			"defaults_json":               jsonAttribute("Default request payload values (`defaults`)."),
			"overrides_json":              jsonAttribute("Request payload values that override the client (`overrides`)."),
			"transformation_json":         jsonAttribute("CEL transformations of the LLM request payload (`transformation`)."),
			"final_transformation_json":   jsonAttribute("CEL transformations applied after conversion to the provider format (`finalTransformation`)."),
			"request_transformation_json": jsonAttribute("CEL transformations of the HTTP request to the provider (`requestTransformation`)."),
			"request_headers_json":        jsonAttribute("Header modifier applied to requests to the provider (`requestHeaders`)."),
			"response_headers_json":       jsonAttribute("Header modifier applied to responses from the provider (`responseHeaders`)."),
			"backend_tls_json":            jsonAttribute("TLS settings for the connection to the provider (`backendTLS`)."),
			"auth_json":                   jsonAttribute("Authentication settings for the connection to the provider (`auth`)."),
			"health_json":                 jsonAttribute("Outlier detection for this model backend (`health`)."),
			"backend_tunnel_json":         jsonAttribute("Tunneling settings for the connection to the provider (`backendTunnel`)."),
			"guardrails_json":             jsonAttribute("Guardrails applied to requests or responses (`guardrails`)."),
			"prompt_caching_json":         jsonAttribute("Cache point insertion settings (`promptCaching`)."),
			"revision": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Server revision of the model.",
			},
		},
	}
}

// jsonAttribute returns an optional normalized-JSON attribute. JSON leaves are
// Sensitive because they can carry credentials: custom provider overrides,
// HTTP authorization, backend auth/TLS and header modifiers (spec §12).
func jsonAttribute(description string) schema.StringAttribute {
	return jsonAttributeWithValidators(description)
}

// jsonAttributeWithValidators returns an optional normalized-JSON attribute
// carrying the given validators.
func jsonAttributeWithValidators(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Sensitive:           true,
		CustomType:          jsontypes.NormalizedType{},
		MarkdownDescription: description,
		Validators:          validators,
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

// model is the Terraform state shape of agentgateway_llm_model. The framework
// populates it by reflection, so only exported fields with tfsdk tags survive
// a plan or state read.
type model struct {
	Name                      types.String         `tfsdk:"name"`
	IDValue                   types.String         `tfsdk:"id"`
	DisplayName               types.String         `tfsdk:"display_name"`
	Visibility                types.String         `tfsdk:"visibility"`
	ProviderName              types.String         `tfsdk:"provider_name"`
	ProviderReference         types.String         `tfsdk:"provider_reference"`
	CustomProviderJSON        jsontypes.Normalized `tfsdk:"custom_provider_json"`
	Model                     types.String         `tfsdk:"model"`
	APIKey                    types.String         `tfsdk:"api_key"`
	BaseURL                   types.String         `tfsdk:"base_url"`
	HostOverride              types.String         `tfsdk:"host_override"`
	PathOverride              types.String         `tfsdk:"path_override"`
	PathPrefix                types.String         `tfsdk:"path_prefix"`
	AWSRegion                 types.String         `tfsdk:"aws_region"`
	VertexRegion              types.String         `tfsdk:"vertex_region"`
	VertexProject             types.String         `tfsdk:"vertex_project"`
	AzureResourceName         types.String         `tfsdk:"azure_resource_name"`
	AzureResourceType         types.String         `tfsdk:"azure_resource_type"`
	AzureAPIVersion           types.String         `tfsdk:"azure_api_version"`
	AzureProjectName          types.String         `tfsdk:"azure_project_name"`
	Tokenize                  types.Bool           `tfsdk:"tokenize"`
	PassthroughJSON           jsontypes.Normalized `tfsdk:"passthrough_json"`
	AuthorizationJSON         jsontypes.Normalized `tfsdk:"authorization_json"`
	MatchesJSON               jsontypes.Normalized `tfsdk:"matches_json"`
	DefaultsJSON              jsontypes.Normalized `tfsdk:"defaults_json"`
	OverridesJSON             jsontypes.Normalized `tfsdk:"overrides_json"`
	TransformationJSON        jsontypes.Normalized `tfsdk:"transformation_json"`
	FinalTransformationJSON   jsontypes.Normalized `tfsdk:"final_transformation_json"`
	RequestTransformationJSON jsontypes.Normalized `tfsdk:"request_transformation_json"`
	RequestHeadersJSON        jsontypes.Normalized `tfsdk:"request_headers_json"`
	ResponseHeadersJSON       jsontypes.Normalized `tfsdk:"response_headers_json"`
	BackendTLSJSON            jsontypes.Normalized `tfsdk:"backend_tls_json"`
	AuthJSON                  jsontypes.Normalized `tfsdk:"auth_json"`
	HealthJSON                jsontypes.Normalized `tfsdk:"health_json"`
	BackendTunnelJSON         jsontypes.Normalized `tfsdk:"backend_tunnel_json"`
	GuardrailsJSON            jsontypes.Normalized `tfsdk:"guardrails_json"`
	PromptCachingJSON         jsontypes.Normalized `tfsdk:"prompt_caching_json"`
	Revision                  types.Int64          `tfsdk:"revision"`
}

// Kind returns the config resource kind.
func (m *model) Kind() configresource.Kind { return configresource.KindLLMModel }

// ID returns the explicit id, else the name (spec §7.0). An id that is unknown
// at plan time falls back to the name; the server returns the stored id.
func (m *model) ID() string {
	if setString(m.IDValue) && m.IDValue.ValueString() != "" {
		return m.IDValue.ValueString()
	}
	return m.Name.ValueString()
}

// SetServerMeta stores the computed id and revision returned by the server.
func (m *model) SetServerMeta(id string, revision int64) {
	m.IDValue = types.StringValue(id)
	m.Revision = types.Int64Value(revision)
}

// Value builds the API value object from the model.
func (m *model) Value(_ context.Context, diags *diag.Diagnostics) json.RawMessage {
	if m.Name.IsNull() || m.Name.IsUnknown() || m.Name.ValueString() == "" {
		diags.AddError("Invalid agentgateway_llm_model", "name is required and must be a non-empty string")
		return nil
	}

	value := map[string]any{"name": m.Name.ValueString()}
	if setString(m.IDValue) && m.IDValue.ValueString() != "" {
		value["id"] = m.IDValue.ValueString()
	}
	// display_name is Terraform-side only: agentgateway v1.7.0 has no
	// displayName on llm.models[] (the display name comes from the model
	// catalog), so sending it makes the gateway reject the whole value with
	// "unknown field `displayName`". It stays in state and is preserved on
	// read like any attribute the server did not return.
	putString(value, "visibility", m.Visibility)

	provider, ok := m.providerValue(diags)
	if !ok {
		return nil
	}
	value["provider"] = provider

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

	for _, leaf := range m.jsonLeaves() {
		if presentJSON(*leaf.value) {
			value[leaf.key] = json.RawMessage(leaf.value.ValueString())
		}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		diags.AddError("Invalid agentgateway_llm_model", "the model could not be encoded as JSON")
		return nil
	}
	return raw
}

// providerValue encodes the provider union (spec C16). Exactly one option is
// required; the schema validators catch the common case and this check is the
// belt-and-braces guard the harness relies on.
func (m *model) providerValue(diags *diag.Diagnostics) (any, bool) {
	set := 0
	for _, present := range []bool{
		setString(m.ProviderName),
		setString(m.ProviderReference),
		setJSON(m.CustomProviderJSON),
	} {
		if present {
			set++
		}
	}

	switch set {
	case 1:
	case 0:
		diags.AddError(
			"Invalid agentgateway_llm_model",
			"exactly one of provider_name, provider_reference or custom_provider_json must be set",
		)
		return nil, false
	default:
		diags.AddError(
			"Invalid agentgateway_llm_model",
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
		raw := m.CustomProviderJSON.ValueString()
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
			diags.AddError(
				"Invalid agentgateway_llm_model",
				"custom_provider_json must be a JSON object",
			)
			return nil, false
		}
		return map[string]any{"custom": json.RawMessage(raw)}, true
	}
}

// SetValue stores the model returned by the gateway. Attributes the server did
// not return keep the value already in state; JSON leaves the plan left null
// stay null (see preserveJSONLeaf).
func (m *model) SetValue(_ context.Context, value json.RawMessage, diags *diag.Diagnostics) {
	if len(value) == 0 {
		return
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(value, &body); err != nil || body == nil {
		diags.AddError("Failed to read agentgateway_llm_model", "the gateway returned a value that is not a JSON object")
		return
	}

	setStringField(body, "name", &m.Name)
	setStringField(body, "id", &m.IDValue)
	setStringField(body, "displayName", &m.DisplayName)
	setStringField(body, "visibility", &m.Visibility)

	if raw, ok := body["provider"]; ok {
		m.setProvider(raw, diags)
		if diags.HasError() {
			return
		}
	}

	if raw, ok := body["params"]; ok && isPresent(raw) {
		var params map[string]json.RawMessage
		if err := json.Unmarshal(raw, &params); err != nil || params == nil {
			diags.AddError("Failed to read agentgateway_llm_model", "the gateway returned params that are not a JSON object")
			return
		}
		m.setParams(params)
	}

	for _, leaf := range m.jsonLeaves() {
		raw, ok := body[leaf.key]
		// The gateway accepts the deprecated `tls` spelling for backendTLS.
		if !ok && leaf.key == "backendTLS" {
			raw, ok = body["tls"]
		}
		if !ok || !isPresent(raw) {
			continue
		}
		*leaf.value = preserveJSONLeaf(*leaf.value, raw)
	}
}

// setParams stores the params the server returned. Missing keys are left as
// they are so an omitted attribute is never zeroed.
func (m *model) setParams(params map[string]json.RawMessage) {
	setStringField(params, "model", &m.Model)
	setStringField(params, "apiKey", &m.APIKey)
	setStringField(params, "baseUrl", &m.BaseURL)
	setStringField(params, "hostOverride", &m.HostOverride)
	setStringField(params, "pathOverride", &m.PathOverride)
	setStringField(params, "pathPrefix", &m.PathPrefix)
	setStringField(params, "awsRegion", &m.AWSRegion)
	setStringField(params, "vertexRegion", &m.VertexRegion)
	setStringField(params, "vertexProject", &m.VertexProject)
	setStringField(params, "azureResourceName", &m.AzureResourceName)
	setStringField(params, "azureResourceType", &m.AzureResourceType)
	setStringField(params, "azureApiVersion", &m.AzureAPIVersion)
	setStringField(params, "azureProjectName", &m.AzureProjectName)

	if raw, ok := params["tokenize"]; ok && isPresent(raw) {
		var tokenize bool
		if err := json.Unmarshal(raw, &tokenize); err == nil {
			m.Tokenize = types.BoolValue(tokenize)
		}
	}
}

// setProvider decodes the provider union: a string, `{reference: name}` or
// `{custom: {...}}`. The three attributes are cleared as a group so state can
// never carry two options at once.
func (m *model) setProvider(raw json.RawMessage, diags *diag.Diagnostics) {
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
	if err := json.Unmarshal(raw, &union); err != nil || union == nil {
		diags.AddError("Failed to read agentgateway_llm_model", "the gateway returned a provider value that is neither a name nor an object")
		return
	}

	if reference, ok := union["reference"]; ok && isPresent(reference) {
		var referenceName string
		if err := json.Unmarshal(reference, &referenceName); err != nil {
			diags.AddError("Failed to read agentgateway_llm_model", "the gateway returned a provider reference that is not a string")
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

	diags.AddError("Failed to read agentgateway_llm_model", "the gateway returned a provider object that is neither a reference nor a custom provider")
}

// jsonLeaf pairs a Terraform JSON attribute with its API field name.
type jsonLeaf struct {
	key   string
	value *jsontypes.Normalized
}

// jsonLeaves lists every JSON leaf in the order the docs present them.
func (m *model) jsonLeaves() []jsonLeaf {
	return []jsonLeaf{
		{"passthrough", &m.PassthroughJSON},
		{"authorization", &m.AuthorizationJSON},
		{"matches", &m.MatchesJSON},
		{"defaults", &m.DefaultsJSON},
		{"overrides", &m.OverridesJSON},
		{"transformation", &m.TransformationJSON},
		{"finalTransformation", &m.FinalTransformationJSON},
		{"requestTransformation", &m.RequestTransformationJSON},
		{"requestHeaders", &m.RequestHeadersJSON},
		{"responseHeaders", &m.ResponseHeadersJSON},
		{"backendTLS", &m.BackendTLSJSON},
		{"auth", &m.AuthJSON},
		{"health", &m.HealthJSON},
		{"backendTunnel", &m.BackendTunnelJSON},
		{"guardrails", &m.GuardrailsJSON},
		{"promptCaching", &m.PromptCachingJSON},
	}
}

// preserveJSONLeaf keeps a JSON leaf the plan or state left null. The gateway
// materializes defaults for some leaves (guardrails, for example), and adopting
// one would store a value the configuration does not contain, so an unset leaf
// stays null rather than becoming an empty object. A leaf the user configured
// is refreshed from the server so normalization and out-of-band edits still
// round-trip.
func preserveJSONLeaf(existing jsontypes.Normalized, raw json.RawMessage) jsontypes.Normalized {
	if existing.IsNull() || existing.IsUnknown() {
		return existing
	}
	return jsontypes.NewNormalizedValue(string(raw))
}

// setStringField decodes one string when the server returned it.
func setStringField(body map[string]json.RawMessage, key string, target *types.String) {
	raw, ok := body[key]
	if !ok || !isPresent(raw) {
		return
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	*target = types.StringValue(value)
}

// putString adds a value when the Terraform value is set.
func putString(body map[string]any, key string, value types.String) {
	if !setString(value) {
		return
	}
	body[key] = value.ValueString()
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
