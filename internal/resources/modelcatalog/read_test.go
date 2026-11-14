package modelcatalog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// TestModelCatalogSetValue proves a server value round-trips into the model and
// that a leaf the server omitted is preserved instead of being zeroed
// (spec §7.15).
func TestModelCatalogSetValue(t *testing.T) {
	t.Parallel()

	t.Run("custom overlay round-trips", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"custom": {"providers": {"openai": {"models": {"gpt-4o": {"rates": {"input": "2.5"}}}}}}
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		assertJSONEqual(t, json.RawMessage(model.CustomJSON.ValueString()), `{"providers":{"openai":{"models":{"gpt-4o":{"rates":{"input":"2.5"}}}}}}`)
		if !model.BaseJSON.IsNull() {
			t.Errorf("base_json = %v, want null", model.BaseJSON)
		}
	})

	t.Run("base catalog round-trips", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"base": {"metadata": {"generatedAt": "2026-11-14T00:00:00Z"}, "providers": {}}
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		assertJSONEqual(t, json.RawMessage(model.BaseJSON.ValueString()), `{"metadata":{"generatedAt":"2026-11-14T00:00:00Z"},"providers":{}}`)
		if !model.CustomJSON.IsNull() {
			t.Errorf("custom_json = %v, want null", model.CustomJSON)
		}
	})

	t.Run("omitted leaf is preserved from state", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		model.CustomJSON = jsontypes.NewNormalizedValue(`{"providers":{"internal":{}}}`)

		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"base": {"providers": {}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		assertJSONEqual(t, json.RawMessage(model.CustomJSON.ValueString()), `{"providers":{"internal":{}}}`)
	})

	t.Run("null leaf on an unknown value becomes null", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		model.CustomJSON = jsontypes.NewNormalizedUnknown()

		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"base": {"providers": {}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.CustomJSON.IsNull() {
			t.Errorf("custom_json = %v, want null so state never carries an unknown", model.CustomJSON)
		}
	})

	t.Run("invalid JSON is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"custom":`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for invalid JSON")
		}
	})

	t.Run("both leaves in the server value are stored", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"base": {"providers": {}},
			"custom": {"providers": {"internal": {}}}
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if model.BaseJSON.IsNull() || model.CustomJSON.IsNull() {
			t.Errorf("base_json = %v, custom_json = %v, want both stored", model.BaseJSON, model.CustomJSON)
		}
	})
}
