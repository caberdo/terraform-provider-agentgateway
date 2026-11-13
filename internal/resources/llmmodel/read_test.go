package llmmodel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestLLMModelReadPreservesUnknownJSON is the required read-back guard: the
// gateway may materialize a JSON leaf the configuration left null (guardrails
// are the common case). Adopting it would make the applied value disagree with
// the plan, so the leaf must stay null rather than become an empty object.
func TestLLMModelReadPreservesUnknownJSON(t *testing.T) {
	t.Parallel()

	values := []struct {
		name  string
		value string
	}{
		{"object", `{"promptInjections":{"action":"block"}}`},
		{"empty object", `{}`},
		{"null", `null`},
	}

	for _, tc := range values {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := testModel()
			var diags diag.Diagnostics
			m.SetValue(
				context.Background(),
				json.RawMessage(`{"name":"gpt-4o","provider":"openai","guardrails":`+tc.value+`}`),
				&diags,
			)
			if diags.HasError() {
				t.Fatalf("SetValue() returned diagnostics: %v", diags)
			}

			if !m.GuardrailsJSON.IsNull() {
				t.Fatalf("guardrails_json = %q, want null", m.GuardrailsJSON.ValueString())
			}
			if got := m.GuardrailsJSON.ValueString(); got == "{}" {
				t.Fatalf("guardrails_json = %q, want null not an empty object", got)
			}
		})
	}
}

// TestLLMModelSetValueRoundTrip proves a server value fills an imported model,
// including the JSON leaves the configuration did set.
func TestLLMModelSetValueRoundTrip(t *testing.T) {
	t.Parallel()

	m := &model{
		// A leaf the plan configured is refreshed from the server, so the
		// normalized form round-trips.
		GuardrailsJSON:    jsontypes.NewNormalizedValue(`{"old":true}`),
		BackendTLSJSON:    jsontypes.NewNormalizedValue(`{"old":true}`),
		PromptCachingJSON: jsontypes.NewNormalizedValue(`{"old":true}`),
	}

	server := json.RawMessage(`{
		"name":"gpt-4o",
		"id":"stable-id",
		"displayName":"GPT-4o",
		"visibility":"internal",
		"provider":{"reference":"shared-anthropic"},
		"params":{
			"model":"gpt-4o-2024-11-20",
			"apiKey":"$OPENAI_API_KEY",
			"baseUrl":"https://api.openai.com/v1",
			"hostOverride":"api.openai.com:443",
			"pathOverride":"/v1/chat/completions",
			"pathPrefix":"/openai",
			"awsRegion":"eu-west-1",
			"vertexRegion":"europe-west4",
			"vertexProject":"my-project",
			"azureResourceName":"my-azure",
			"azureResourceType":"openAI",
			"azureApiVersion":"2024-10-21",
			"azureProjectName":"my-foundry",
			"tokenize":true
		},
		"guardrails":{"promptInjections":{"action":"block"}},
		"backendTLS":{"insecureSkipVerify":true},
		"promptCaching":{"enabled":true}
	}`)

	var diags diag.Diagnostics
	m.SetValue(context.Background(), server, &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() returned diagnostics: %v", diags)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"name", m.Name.ValueString(), "gpt-4o"},
		{"id", m.IDValue.ValueString(), "stable-id"},
		{"display_name", m.DisplayName.ValueString(), "GPT-4o"},
		{"visibility", m.Visibility.ValueString(), "internal"},
		{"model", m.Model.ValueString(), "gpt-4o-2024-11-20"},
		{"api_key", m.APIKey.ValueString(), "$OPENAI_API_KEY"},
		{"base_url", m.BaseURL.ValueString(), "https://api.openai.com/v1"},
		{"host_override", m.HostOverride.ValueString(), "api.openai.com:443"},
		{"path_override", m.PathOverride.ValueString(), "/v1/chat/completions"},
		{"path_prefix", m.PathPrefix.ValueString(), "/openai"},
		{"aws_region", m.AWSRegion.ValueString(), "eu-west-1"},
		{"vertex_region", m.VertexRegion.ValueString(), "europe-west4"},
		{"vertex_project", m.VertexProject.ValueString(), "my-project"},
		{"azure_resource_name", m.AzureResourceName.ValueString(), "my-azure"},
		{"azure_resource_type", m.AzureResourceType.ValueString(), "openAI"},
		{"azure_api_version", m.AzureAPIVersion.ValueString(), "2024-10-21"},
		{"azure_project_name", m.AzureProjectName.ValueString(), "my-foundry"},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %q, want %q", check.name, check.got, check.want)
		}
	}

	if !m.Tokenize.ValueBool() {
		t.Error("tokenize = false, want true")
	}
	if got := m.ProviderReference.ValueString(); got != "shared-anthropic" {
		t.Errorf("provider_reference = %q, want shared-anthropic", got)
	}
	if !m.ProviderName.IsNull() || !m.CustomProviderJSON.IsNull() {
		t.Error("provider and custom_provider_json must be cleared when the server returns a reference")
	}
	if got := m.GuardrailsJSON.ValueString(); got != `{"promptInjections":{"action":"block"}}` {
		t.Errorf("guardrails_json = %q, want the server value", got)
	}
	if got := m.BackendTLSJSON.ValueString(); got != `{"insecureSkipVerify":true}` {
		t.Errorf("backend_tls_json = %q, want the server value", got)
	}
	if got := m.PromptCachingJSON.ValueString(); got != `{"enabled":true}` {
		t.Errorf("prompt_caching_json = %q, want the server value", got)
	}
}

// TestLLMModelSetValuePreservesOmitted proves attributes the server did not
// return keep the plan or state value instead of being zeroed.
func TestLLMModelSetValuePreservesOmitted(t *testing.T) {
	t.Parallel()

	m := &model{
		Name:           types.StringValue("from-state"),
		DisplayName:    types.StringValue("keep me"),
		Model:          types.StringValue("old-model"),
		APIKey:         types.StringValue("$KEEP"),
		Tokenize:       types.BoolValue(true),
		GuardrailsJSON: jsontypes.NewNormalizedValue(`{"keep":true}`),
	}

	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"name":"from-server"}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() returned diagnostics: %v", diags)
	}

	if got := m.Name.ValueString(); got != "from-server" {
		t.Errorf("name = %q, want from-server", got)
	}
	if got := m.DisplayName.ValueString(); got != "keep me" {
		t.Errorf("display_name = %q, want the state value", got)
	}
	if got := m.Model.ValueString(); got != "old-model" {
		t.Errorf("model = %q, want the state value", got)
	}
	if got := m.APIKey.ValueString(); got != "$KEEP" {
		t.Errorf("api_key = %q, want the state value", got)
	}
	if !m.Tokenize.ValueBool() {
		t.Error("tokenize was zeroed")
	}
	if got := m.GuardrailsJSON.ValueString(); got != `{"keep":true}` {
		t.Errorf("guardrails_json = %q, want the state value", got)
	}
}

// TestLLMModelSetValueProviderUnion covers the three provider shapes the
// gateway can return; the group is cleared so state never carries two.
func TestLLMModelSetValueProviderUnion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		check func(*testing.T, *model)
	}{
		{
			name:  "built-in name",
			value: `{"name":"gpt-4o","provider":"openai"}`,
			check: func(t *testing.T, m *model) {
				t.Helper()
				if got := m.ProviderName.ValueString(); got != "openai" {
					t.Errorf("provider = %q, want openai", got)
				}
				if !m.ProviderReference.IsNull() || !m.CustomProviderJSON.IsNull() {
					t.Error("reference and custom must be null for a built-in provider")
				}
			},
		},
		{
			name:  "custom provider",
			value: `{"name":"gpt-4o","provider":{"custom":{"model":"m","formats":["completions"]}}}`,
			check: func(t *testing.T, m *model) {
				t.Helper()
				if got := m.CustomProviderJSON.ValueString(); got != `{"model":"m","formats":["completions"]}` {
					t.Errorf("custom_provider_json = %q", got)
				}
				if !m.ProviderName.IsNull() || !m.ProviderReference.IsNull() {
					t.Error("provider and reference must be null for a custom provider")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &model{
				ProviderName:      types.StringValue("stale"),
				ProviderReference: types.StringValue("stale"),
			}
			var diags diag.Diagnostics
			m.SetValue(context.Background(), json.RawMessage(tc.value), &diags)
			if diags.HasError() {
				t.Fatalf("SetValue() returned diagnostics: %v", diags)
			}
			tc.check(t, m)
		})
	}
}

// TestLLMModelSetValueReadsLegacyTLS proves the deprecated `tls` spelling the
// gateway accepts is read into backend_tls_json as well.
func TestLLMModelSetValueReadsLegacyTLS(t *testing.T) {
	t.Parallel()

	m := &model{BackendTLSJSON: jsontypes.NewNormalizedValue(`{"old":true}`)}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`{"name":"gpt-4o","tls":{"insecureSkipVerify":true}}`), &diags)
	if diags.HasError() {
		t.Fatalf("SetValue() returned diagnostics: %v", diags)
	}

	if got := m.BackendTLSJSON.ValueString(); got != `{"insecureSkipVerify":true}` {
		t.Errorf("backend_tls_json = %q, want the legacy tls value", got)
	}
}

func TestLLMModelSetValueRejectsNonObject(t *testing.T) {
	t.Parallel()

	m := &model{Name: types.StringValue("keep")}
	var diags diag.Diagnostics
	m.SetValue(context.Background(), json.RawMessage(`["not","an","object"]`), &diags)
	if !diags.HasError() {
		t.Fatal("SetValue() accepted a non-object value")
	}
	if got := m.Name.ValueString(); got != "keep" {
		t.Errorf("name = %q, want the previous value", got)
	}
}

func TestLLMModelSetServerMeta(t *testing.T) {
	t.Parallel()

	m := &model{}
	m.SetServerMeta("stable-id", 12)

	if got := m.IDValue.ValueString(); got != "stable-id" {
		t.Errorf("id = %q, want stable-id", got)
	}
	if got := m.Revision.ValueInt64(); got != 12 {
		t.Errorf("revision = %d, want 12", got)
	}
}
