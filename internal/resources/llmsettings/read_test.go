package llmsettings

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestLLMSettingsSetValue proves a server value round-trips into the model and
// that fields the config file owns stay exactly as they were in state.
func TestLLMSettingsSetValue(t *testing.T) {
	t.Parallel()

	t.Run("gateways and tls round-trip", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"gateways": ["default", "llm/listener"],
			"tls": {"cert": "/etc/tls/tls.crt", "key": "/etc/tls/tls.key"}
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		var gateways []string
		if listDiags := model.Gateways.ElementsAs(context.Background(), &gateways, false); listDiags.HasError() {
			t.Fatalf("decoding gateways: %v", listDiags)
		}
		if len(gateways) != 2 || gateways[0] != "default" || gateways[1] != "llm/listener" {
			t.Errorf("gateways = %v, want [default llm/listener]", gateways)
		}
		assertJSONEqual(t, json.RawMessage(model.TLSJSON.ValueString()), `{"cert":"/etc/tls/tls.crt","key":"/etc/tls/tls.key"}`)
		if !model.Port.IsNull() {
			t.Errorf("port = %v, want null", model.Port)
		}
	})

	t.Run("an empty gateways list stays an empty list", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"gateways":[]}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if model.Gateways.IsNull() {
			t.Fatal("gateways = null, want an empty list")
		}
		if got := len(model.Gateways.Elements()); got != 0 {
			t.Errorf("len(gateways) = %d, want 0", got)
		}
	})

	t.Run("omitted attributes stay null", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Gateways.IsNull() {
			t.Errorf("gateways = %v, want null", model.Gateways)
		}
		if !model.Port.IsNull() {
			t.Errorf("port = %v, want null", model.Port)
		}
		if !model.TLSJSON.IsNull() {
			t.Errorf("tls_json = %v, want null", model.TLSJSON)
		}
	})

	t.Run("file-owned fields omitted by the server are preserved from state", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{
			Gateways: types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("default"),
			}),
			TLSJSON: jsontypes.NewNormalizedValue(`{"cert":"/etc/tls/tls.crt"}`),
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"port":8080}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Port.ValueInt64(); got != 8080 {
			t.Errorf("port = %d, want 8080", got)
		}
		if model.Gateways.IsNull() {
			t.Fatal("gateways = null, want the state value preserved")
		}
		var gateways []string
		if listDiags := model.Gateways.ElementsAs(context.Background(), &gateways, false); listDiags.HasError() {
			t.Fatalf("decoding gateways: %v", listDiags)
		}
		if len(gateways) != 1 || gateways[0] != "default" {
			t.Errorf("gateways = %v, want the state value [default]", gateways)
		}
		assertJSONEqual(t, json.RawMessage(model.TLSJSON.ValueString()), `{"cert":"/etc/tls/tls.crt"}`)
	})

	t.Run("explicit nulls in the response preserve state", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{
			Gateways: types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("default"),
			}),
			Port: types.Int64Value(8080),
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"gateways":null,"port":null,"tls":null}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if model.Gateways.IsNull() {
			t.Error("gateways = null, want the state value preserved")
		}
		if got := model.Port.ValueInt64(); got != 8080 {
			t.Errorf("port = %d, want the state value 8080", got)
		}
	})

	t.Run("invalid JSON is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"gateways":`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for invalid JSON")
		}
	})

	t.Run("a gateways value that is not a list is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := &llmSettingsModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"gateways":"default"}`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for a gateways value that is not a list")
		}
	})
}

// TestLLMSettingsSetServerMeta covers the computed attributes the harness
// stores after a write.
func TestLLMSettingsSetServerMeta(t *testing.T) {
	t.Parallel()

	model := &llmSettingsModel{}
	model.SetServerMeta("default", 4)

	if got := model.IDValue.ValueString(); got != "default" {
		t.Errorf("id = %q, want %q", got, "default")
	}
	if got := model.Revision.ValueInt64(); got != 4 {
		t.Errorf("revision = %d, want 4", got)
	}
}
