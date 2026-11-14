package llmmodel

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// testModel is the smallest valid model: a name, the default visibility and a
// built-in provider. Every JSON leaf is null, so the expected payload contains
// no JSON key at all.
func testModel() *model {
	return &model{
		Name:         types.StringValue("gpt-4o"),
		Visibility:   types.StringValue("public"),
		ProviderName: types.StringValue("openai"),
	}
}

func mustValue(t *testing.T, m *model) map[string]any {
	t.Helper()

	var diags diag.Diagnostics
	raw := m.Value(context.Background(), &diags)
	if diags.HasError() {
		t.Fatalf("Value() returned diagnostics: %v", diags)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Value() returned invalid JSON %q: %v", raw, err)
	}
	return got
}

func TestLLMModelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model func() *model
		want  map[string]any
	}{
		{
			name:  "minimal",
			model: testModel,
			want: map[string]any{
				"name":       "gpt-4o",
				"provider":   "openai",
				"visibility": "public",
			},
		},
		{
			name: "explicit_id",
			model: func() *model {
				m := testModel()
				m.IDValue = types.StringValue("stable-id")
				return m
			},
			want: map[string]any{
				"name":       "gpt-4o",
				"id":         "stable-id",
				"provider":   "openai",
				"visibility": "public",
			},
		},
		{
			// display_name is Terraform-side only: the gateway has no
			// displayName field, so it must never reach the wire.
			name: "internal_visibility_omits_display_name",
			model: func() *model {
				m := testModel()
				m.Visibility = types.StringValue("internal")
				m.DisplayName = types.StringValue("GPT-4o")
				return m
			},
			want: map[string]any{
				"name":       "gpt-4o",
				"provider":   "openai",
				"visibility": "internal",
			},
		},
		{
			name: "provider_reference",
			model: func() *model {
				m := testModel()
				m.ProviderName = types.StringNull()
				m.ProviderReference = types.StringValue("shared-anthropic")
				return m
			},
			want: map[string]any{
				"name":       "gpt-4o",
				"provider":   map[string]any{"reference": "shared-anthropic"},
				"visibility": "public",
			},
		},
		{
			name: "custom_provider",
			model: func() *model {
				m := testModel()
				m.ProviderName = types.StringNull()
				m.CustomProviderJSON = jsontypes.NewNormalizedValue(`{"model":"m","formats":["completions"]}`)
				return m
			},
			want: map[string]any{
				"name": "gpt-4o",
				"provider": map[string]any{
					"custom": map[string]any{
						"model":   "m",
						"formats": []any{"completions"},
					},
				},
				"visibility": "public",
			},
		},
		{
			name: "typed_params",
			model: func() *model {
				m := testModel()
				m.Model = types.StringValue("gpt-4o-2024-11-20")
				m.APIKey = types.StringValue("$OPENAI_API_KEY")
				m.BaseURL = types.StringValue("https://api.openai.com/v1")
				m.HostOverride = types.StringValue("api.openai.com:443")
				m.PathOverride = types.StringValue("/v1/chat/completions")
				m.PathPrefix = types.StringValue("/openai")
				m.AWSRegion = types.StringValue("eu-west-1")
				m.VertexRegion = types.StringValue("europe-west4")
				m.VertexProject = types.StringValue("my-project")
				m.AzureResourceName = types.StringValue("my-azure")
				m.AzureResourceType = types.StringValue("openAI")
				m.AzureAPIVersion = types.StringValue("2024-10-21")
				m.AzureProjectName = types.StringValue("my-foundry")
				m.Tokenize = types.BoolValue(true)
				return m
			},
			want: map[string]any{
				"name":       "gpt-4o",
				"provider":   "openai",
				"visibility": "public",
				"params": map[string]any{
					"model":             "gpt-4o-2024-11-20",
					"apiKey":            "$OPENAI_API_KEY",
					"baseUrl":           "https://api.openai.com/v1",
					"hostOverride":      "api.openai.com:443",
					"pathOverride":      "/v1/chat/completions",
					"pathPrefix":        "/openai",
					"awsRegion":         "eu-west-1",
					"vertexRegion":      "europe-west4",
					"vertexProject":     "my-project",
					"azureResourceName": "my-azure",
					"azureResourceType": "openAI",
					"azureApiVersion":   "2024-10-21",
					"azureProjectName":  "my-foundry",
					"tokenize":          true,
				},
			},
		},
		{
			name: "tokenize_false_is_sent",
			model: func() *model {
				m := testModel()
				m.Tokenize = types.BoolValue(false)
				return m
			},
			want: map[string]any{
				"name":       "gpt-4o",
				"provider":   "openai",
				"visibility": "public",
				"params":     map[string]any{"tokenize": false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mustValue(t, tc.model()); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected value\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}

// TestLLMModelValueJSONLeaves proves that every *_json attribute maps to its
// upstream field name and is omitted entirely when null.
func TestLLMModelValueJSONLeaves(t *testing.T) {
	t.Parallel()

	jsonValue := `{"k":"v"}`
	decoded := map[string]any{"k": "v"}

	tests := []struct {
		field string
		key   string
		set   func(*model, jsontypes.Normalized)
	}{
		{"passthrough_json", "passthrough", func(m *model, v jsontypes.Normalized) { m.PassthroughJSON = v }},
		{"authorization_json", "authorization", func(m *model, v jsontypes.Normalized) { m.AuthorizationJSON = v }},
		{"matches_json", "matches", func(m *model, v jsontypes.Normalized) { m.MatchesJSON = v }},
		{"defaults_json", "defaults", func(m *model, v jsontypes.Normalized) { m.DefaultsJSON = v }},
		{"overrides_json", "overrides", func(m *model, v jsontypes.Normalized) { m.OverridesJSON = v }},
		{"transformation_json", "transformation", func(m *model, v jsontypes.Normalized) { m.TransformationJSON = v }},
		{"final_transformation_json", "finalTransformation", func(m *model, v jsontypes.Normalized) { m.FinalTransformationJSON = v }},
		{"request_transformation_json", "requestTransformation", func(m *model, v jsontypes.Normalized) { m.RequestTransformationJSON = v }},
		{"request_headers_json", "requestHeaders", func(m *model, v jsontypes.Normalized) { m.RequestHeadersJSON = v }},
		{"response_headers_json", "responseHeaders", func(m *model, v jsontypes.Normalized) { m.ResponseHeadersJSON = v }},
		{"backend_tls_json", "backendTLS", func(m *model, v jsontypes.Normalized) { m.BackendTLSJSON = v }},
		{"auth_json", "auth", func(m *model, v jsontypes.Normalized) { m.AuthJSON = v }},
		{"health_json", "health", func(m *model, v jsontypes.Normalized) { m.HealthJSON = v }},
		{"backend_tunnel_json", "backendTunnel", func(m *model, v jsontypes.Normalized) { m.BackendTunnelJSON = v }},
		{"guardrails_json", "guardrails", func(m *model, v jsontypes.Normalized) { m.GuardrailsJSON = v }},
		{"prompt_caching_json", "promptCaching", func(m *model, v jsontypes.Normalized) { m.PromptCachingJSON = v }},
	}

	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			t.Parallel()

			m := testModel()
			tc.set(m, jsontypes.NewNormalizedValue(jsonValue))

			want := map[string]any{
				"name":       "gpt-4o",
				"provider":   "openai",
				"visibility": "public",
				tc.key:       decoded,
			}
			if got := mustValue(t, m); !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected value\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestLLMModelValueRequiresExactlyOneProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model func() *model
	}{
		{
			name: "none",
			model: func() *model {
				m := testModel()
				m.ProviderName = types.StringNull()
				return m
			},
		},
		{
			name: "two",
			model: func() *model {
				m := testModel()
				m.ProviderReference = types.StringValue("shared")
				return m
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var diags diag.Diagnostics
			raw := tc.model().Value(context.Background(), &diags)
			if !diags.HasError() {
				t.Fatalf("expected an error diagnostic, got value %s", raw)
			}
		})
	}
}

func TestLLMModelValueRejectsNonObjectCustomProvider(t *testing.T) {
	t.Parallel()

	m := testModel()
	m.ProviderName = types.StringNull()
	m.CustomProviderJSON = jsontypes.NewNormalizedValue(`"not-an-object"`)

	var diags diag.Diagnostics
	if raw := m.Value(context.Background(), &diags); !diags.HasError() {
		t.Fatalf("expected an error diagnostic, got value %s", raw)
	}
}

func TestLLMModelID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model *model
		want  string
	}{
		{"id_wins", &model{Name: types.StringValue("pattern/*"), IDValue: types.StringValue("stable")}, "stable"},
		{"name_when_id_null", &model{Name: types.StringValue("gpt-4o"), IDValue: types.StringNull()}, "gpt-4o"},
		{"name_when_id_unknown", &model{Name: types.StringValue("gpt-4o"), IDValue: types.StringUnknown()}, "gpt-4o"},
		{"name_when_id_empty", &model{Name: types.StringValue("gpt-4o"), IDValue: types.StringValue("")}, "gpt-4o"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.model.ID(); got != tc.want {
				t.Fatalf("ID() = %q, want %q", got, tc.want)
			}
		})
	}
}
