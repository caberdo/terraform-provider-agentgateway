package llmprovider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestLLMProviderSetValue proves a server value round-trips into the model and
// that attributes the server omitted stay null instead of being zeroed.
func TestLLMProviderSetValue(t *testing.T) {
	t.Parallel()

	t.Run("full value round-trips", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"name": "openai-prod",
			"provider": "openAI",
			"params": {
				"model": "gpt-4o",
				"apiKey": "$OPENAI_API_KEY",
				"baseUrl": "https://api.openai.com",
				"hostOverride": "llm.example.com",
				"pathOverride": "/v1/chat",
				"pathPrefix": "/openai",
				"awsRegion": "eu-west-1",
				"vertexRegion": "europe-west4",
				"vertexProject": "my-project",
				"azureResourceName": "my-resource",
				"azureResourceType": "openAI",
				"azureApiVersion": "2024-10-21",
				"azureProjectName": "my-azure-project",
				"tokenize": true
			},
			"defaults": {"temperature": 0.2}
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Name.ValueString(); got != "openai-prod" {
			t.Errorf("name = %q, want %q", got, "openai-prod")
		}
		if got := model.ProviderName.ValueString(); got != "openAI" {
			t.Errorf("provider = %q, want %q", got, "openAI")
		}
		if !model.ProviderReference.IsNull() {
			t.Errorf("provider_reference = %v, want null", model.ProviderReference)
		}
		if !model.CustomProviderJSON.IsNull() {
			t.Errorf("custom_provider_json = %v, want null", model.CustomProviderJSON)
		}
		for name, got := range map[string]types.String{
			"model":               model.Model,
			"api_key":             model.APIKey,
			"base_url":            model.BaseURL,
			"host_override":       model.HostOverride,
			"path_override":       model.PathOverride,
			"path_prefix":         model.PathPrefix,
			"aws_region":          model.AWSRegion,
			"vertex_region":       model.VertexRegion,
			"vertex_project":      model.VertexProject,
			"azure_resource_name": model.AzureResourceName,
			"azure_resource_type": model.AzureResourceType,
			"azure_api_version":   model.AzureAPIVersion,
			"azure_project_name":  model.AzureProjectName,
		} {
			if got.IsNull() {
				t.Errorf("%s is null, want the server value", name)
			}
		}
		if !model.Tokenize.ValueBool() {
			t.Error("tokenize = false, want true")
		}
		assertJSONEqual(t, json.RawMessage(model.DefaultsJSON.ValueString()), `{"temperature":0.2}`)
	})

	t.Run("omitted params and defaults stay null", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"bare","provider":"openai"}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Model.IsNull() {
			t.Errorf("model = %v, want null", model.Model)
		}
		if !model.APIKey.IsNull() {
			t.Errorf("api_key = %v, want null", model.APIKey)
		}
		if !model.Tokenize.IsNull() {
			t.Errorf("tokenize = %v, want null", model.Tokenize)
		}
		if !model.DefaultsJSON.IsNull() {
			t.Errorf("defaults_json = %v, want null", model.DefaultsJSON)
		}
	})

	t.Run("omitted attributes are preserved from state", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{
			Name:     types.StringValue("from-state"),
			Model:    types.StringValue("gpt-4o-mini"),
			APIKey:   types.StringValue("$FROM_STATE"),
			Tokenize: types.BoolValue(true),
			DefaultsJSON: jsontypes.NewNormalizedValue(
				`{"temperature":0.1}`,
			),
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"provider":"openai"}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Name.ValueString(); got != "from-state" {
			t.Errorf("name = %q, want the state value", got)
		}
		if got := model.Model.ValueString(); got != "gpt-4o-mini" {
			t.Errorf("model = %q, want the state value", got)
		}
		if got := model.APIKey.ValueString(); got != "$FROM_STATE" {
			t.Errorf("api_key = %q, want the state value", got)
		}
		if !model.Tokenize.ValueBool() {
			t.Error("tokenize = false, want the state value true")
		}
		assertJSONEqual(t, json.RawMessage(model.DefaultsJSON.ValueString()), `{"temperature":0.1}`)
	})

	t.Run("reference round-trip clears the other provider options", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{
			ProviderName:       types.StringValue("openai"),
			CustomProviderJSON: jsontypes.NewNormalizedValue(`{"model":"old"}`),
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"fast","provider":{"reference":"openai-prod"}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.ProviderReference.ValueString(); got != "openai-prod" {
			t.Errorf("provider_reference = %q, want %q", got, "openai-prod")
		}
		if !model.ProviderName.IsNull() {
			t.Errorf("provider_name = %v, want null", model.ProviderName)
		}
		if !model.CustomProviderJSON.IsNull() {
			t.Errorf("custom_provider_json = %v, want null", model.CustomProviderJSON)
		}
	})

	t.Run("custom round-trip clears the other provider options", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{
			ProviderName:      types.StringValue("openai"),
			ProviderReference: types.StringValue("openai-prod"),
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"self-hosted","provider":{"custom":{"model":"llama3"}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if want := jsontypes.NewNormalizedValue(`{"model":"llama3"}`); !model.CustomProviderJSON.Equal(want) {
			t.Errorf("custom_provider_json = %v, want %v", model.CustomProviderJSON, want)
		}
		if !model.ProviderName.IsNull() {
			t.Errorf("provider_name = %v, want null", model.ProviderName)
		}
		if !model.ProviderReference.IsNull() {
			t.Errorf("provider_reference = %v, want null", model.ProviderReference)
		}
	})

	t.Run("invalid JSON is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for invalid JSON")
		}
	})

	t.Run("an unknown provider object is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := &llmProviderModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"p","provider":{"unknown":"x"}}`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for an unknown provider object")
		}
	})
}

// TestLLMProviderSetServerMeta covers the computed attributes the harness
// stores after a write.
func TestLLMProviderSetServerMeta(t *testing.T) {
	t.Parallel()

	model := &llmProviderModel{}
	model.SetServerMeta("openai-prod", 9)

	if got := model.IDValue.ValueString(); got != "openai-prod" {
		t.Errorf("id = %q, want %q", got, "openai-prod")
	}
	if got := model.Revision.ValueInt64(); got != 9 {
		t.Errorf("revision = %d, want 9", got)
	}
}
