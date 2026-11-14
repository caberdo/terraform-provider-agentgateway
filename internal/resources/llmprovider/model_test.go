package llmprovider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestLLMProviderValue(t *testing.T) {
	tests := []struct {
		name  string
		model llmProviderModel
		want  string
	}{
		{
			name: "builtin provider with params and defaults",
			model: llmProviderModel{
				Name:         types.StringValue("openai-prod"),
				ProviderName: types.StringValue("openAI"),
				Model:        types.StringValue("gpt-4o"),
				APIKey:       types.StringValue("$OPENAI_API_KEY"),
				BaseURL:      types.StringValue("https://api.openai.com"),
				Tokenize:     types.BoolValue(true),
				DefaultsJSON: jsontypes.NewNormalizedValue(`{"temperature":0.2}`),
			},
			want: `{
				"name": "openai-prod",
				"provider": "openAI",
				"params": {
					"apiKey": "$OPENAI_API_KEY",
					"baseUrl": "https://api.openai.com",
					"model": "gpt-4o",
					"tokenize": true
				},
				"defaults": {"temperature": 0.2}
			}`,
		},
		{
			name: "preset provider with every parameter",
			model: llmProviderModel{
				Name:              types.StringValue("deepseek-eu"),
				ProviderName:      types.StringValue("deepseek"),
				Model:             types.StringValue("deepseek-chat"),
				HostOverride:      types.StringValue("llm.example.com"),
				PathOverride:      types.StringValue("/v1/chat"),
				PathPrefix:        types.StringValue("/deepseek"),
				AWSRegion:         types.StringValue("eu-west-1"),
				VertexRegion:      types.StringValue("europe-west4"),
				VertexProject:     types.StringValue("my-project"),
				AzureResourceName: types.StringValue("my-resource"),
				AzureResourceType: types.StringValue("openAI"),
				AzureAPIVersion:   types.StringValue("2024-10-21"),
				AzureProjectName:  types.StringValue("my-azure-project"),
			},
			want: `{
				"name": "deepseek-eu",
				"provider": "deepseek",
				"params": {
					"model": "deepseek-chat",
					"hostOverride": "llm.example.com",
					"pathOverride": "/v1/chat",
					"pathPrefix": "/deepseek",
					"awsRegion": "eu-west-1",
					"vertexRegion": "europe-west4",
					"vertexProject": "my-project",
					"azureResourceName": "my-resource",
					"azureResourceType": "openAI",
					"azureApiVersion": "2024-10-21",
					"azureProjectName": "my-azure-project"
				}
			}`,
		},
		{
			name: "provider reference is the second provider option",
			model: llmProviderModel{
				Name:              types.StringValue("fast"),
				ProviderReference: types.StringValue("openai-prod"),
			},
			want: `{"name": "fast", "provider": {"reference": "openai-prod"}}`,
		},
		{
			name: "custom provider is the third provider option",
			model: llmProviderModel{
				Name:               types.StringValue("self-hosted"),
				CustomProviderJSON: jsontypes.NewNormalizedValue(`{"model":"llama3","providerOverride":"vllm","formats":[{"type":"completions"}]}`),
			},
			want: `{
				"name": "self-hosted",
				"provider": {
					"custom": {
						"model": "llama3",
						"providerOverride": "vllm",
						"formats": [{"type": "completions"}]
					}
				}
			}`,
		},
		{
			name: "null attributes are omitted from the payload",
			model: llmProviderModel{
				Name:               types.StringValue("bare"),
				ProviderName:       types.StringValue("openai"),
				ProviderReference:  types.StringNull(),
				CustomProviderJSON: jsontypes.NewNormalizedNull(),
				Model:              types.StringNull(),
				APIKey:             types.StringNull(),
				Tokenize:           types.BoolNull(),
				DefaultsJSON:       jsontypes.NewNormalizedNull(),
			},
			want: `{"name": "bare", "provider": "openai"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := tt.model.Value(context.Background(), &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

func TestLLMProviderValueRejectsAmbiguousProviderOptions(t *testing.T) {
	tests := []struct {
		name  string
		model llmProviderModel
	}{
		{
			name: "string and reference",
			model: llmProviderModel{
				Name:              types.StringValue("p"),
				ProviderName:      types.StringValue("openai"),
				ProviderReference: types.StringValue("other"),
			},
		},
		{
			name: "string and custom",
			model: llmProviderModel{
				Name:               types.StringValue("p"),
				ProviderName:       types.StringValue("openai"),
				CustomProviderJSON: jsontypes.NewNormalizedValue(`{"model":"m"}`),
			},
		},
		{
			name: "reference and custom",
			model: llmProviderModel{
				Name:               types.StringValue("p"),
				ProviderReference:  types.StringValue("other"),
				CustomProviderJSON: jsontypes.NewNormalizedValue(`{"model":"m"}`),
			},
		},
		{
			name:  "none",
			model: llmProviderModel{Name: types.StringValue("p")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			got := tt.model.Value(context.Background(), &diags)
			if !diags.HasError() {
				t.Fatal("expected a diagnostic for an invalid provider option combination")
			}
			if got != nil {
				t.Errorf("expected no payload, got %s", got)
			}
		})
	}
}

func TestLLMProviderValueRequiresName(t *testing.T) {
	model := llmProviderModel{ProviderName: types.StringValue("openai")}

	var diags diag.Diagnostics
	if got := model.Value(context.Background(), &diags); got != nil {
		t.Errorf("expected no payload, got %s", got)
	}
	if !diags.HasError() {
		t.Fatal("expected a diagnostic for a missing name")
	}
}

func TestLLMProviderIDAndServerMeta(t *testing.T) {
	model := llmProviderModel{Name: types.StringValue("openai-prod")}
	if got := model.ID(); got != "openai-prod" {
		t.Errorf("expected id %q, got %q", "openai-prod", got)
	}

	model.SetServerMeta("openai-prod", 7)
	if model.IDValue.ValueString() != "openai-prod" {
		t.Errorf("expected stored id, got %q", model.IDValue.ValueString())
	}
	if model.Revision.ValueInt64() != 7 {
		t.Errorf("expected revision 7, got %d", model.Revision.ValueInt64())
	}
}

func TestLLMProviderSchema(t *testing.T) {
	s := newSchema()

	for _, name := range []string{"id", "revision"} {
		attribute, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("expected a %q attribute", name)
		}
		if !attribute.IsComputed() {
			t.Errorf("%q must be computed", name)
		}
		if attribute.IsOptional() || attribute.IsRequired() {
			t.Errorf("%q must be computed only", name)
		}
	}

	apiKey, ok := s.Attributes["api_key"]
	if !ok {
		t.Fatal("expected an api_key attribute")
	}
	if !apiKey.IsSensitive() {
		t.Error("api_key must be sensitive")
	}

	for _, name := range []string{"name", "provider_name", "provider_reference", "custom_provider_json", "model", "base_url"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("expected a %q attribute", name)
		}
	}

	// Terraform reserves `provider` as a root attribute name (it is the
	// provider meta-argument), so the string option must not use it.
	if _, ok := s.Attributes["provider"]; ok {
		t.Error("the schema must not expose a reserved root attribute named provider; use provider_name")
	}
}

func assertJSONEqual(t *testing.T, got json.RawMessage, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decoding value %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decoding want %s: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		canonical, _ := json.Marshal(gotValue)
		t.Errorf("value mismatch:\n got: %s\nwant: %s", canonical, want)
	}
}
