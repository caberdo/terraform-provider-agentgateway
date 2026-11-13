package llmvirtualmodel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestVirtualModelSetValue proves a server value round-trips into the model and
// that attributes the server omitted stay null instead of being zeroed.
func TestVirtualModelSetValue(t *testing.T) {
	t.Parallel()

	t.Run("weighted value round-trips", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"smart","routing":{"weighted":{"targets":[{"model":"a","weight":90},{"model":"b","weight":10}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Name.ValueString(); got != "smart" {
			t.Errorf("name = %q, want %q", got, "smart")
		}
		if len(model.Weighted) != 1 || len(model.Weighted[0].Targets) != 2 {
			t.Fatalf("weighted = %#v, want one block with two targets", model.Weighted)
		}
		if got := model.Weighted[0].Targets[0].Weight.ValueInt64(); got != 90 {
			t.Errorf("first weight = %d, want 90", got)
		}
		if got := model.Weighted[0].Targets[1].Model.ValueString(); got != "b" {
			t.Errorf("second model = %q, want %q", got, "b")
		}
	})

	t.Run("omitted weight stays null", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"smart","routing":{"weighted":{"targets":[{"model":"a"}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Weighted[0].Targets[0].Weight.IsNull() {
			t.Errorf("weight = %v, want null", model.Weighted[0].Targets[0].Weight)
		}
	})

	t.Run("server default weight keeps an unset weight null", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Weighted: []weightedModel{{Targets: []weightedTargetModel{
				{Model: types.StringValue("a")},
			}}},
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"smart","routing":{"weighted":{"targets":[{"model":"a","weight":1}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Weighted[0].Targets[0].Weight.IsNull() {
			t.Errorf("weight = %v, want null so the plan and state agree", model.Weighted[0].Targets[0].Weight)
		}
	})

	t.Run("server weight overrides a null state weight when it is not the default", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Weighted: []weightedModel{{Targets: []weightedTargetModel{
				{Model: types.StringValue("a")},
			}}},
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"smart","routing":{"weighted":{"targets":[{"model":"a","weight":7}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Weighted[0].Targets[0].Weight.ValueInt64(); got != 7 {
			t.Errorf("weight = %d, want 7", got)
		}
	})

	t.Run("server default priority keeps an unset priority null", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Failover: []failoverModel{{Targets: []failoverTargetModel{
				{Model: types.StringValue("a")},
			}}},
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"resilient","routing":{"failover":{"targets":[{"model":"a","priority":0}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Failover[0].Targets[0].Priority.IsNull() {
			t.Errorf("priority = %v, want null", model.Failover[0].Targets[0].Priority)
		}
	})

	t.Run("omitted name is preserved from state", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{Name: types.StringValue("from-state")}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"routing":{"weighted":{"targets":[{"model":"a"}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Name.ValueString(); got != "from-state" {
			t.Errorf("name = %q, want %q", got, "from-state")
		}
	})

	t.Run("omitted conditional when is preserved from state", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Conditional: []conditionalModel{{Targets: []conditionalTargetModel{
				{Model: types.StringValue("a"), When: types.StringValue("request.headers[\"x\"] == \"1\"")},
				{Model: types.StringValue("b")},
			}}},
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"adaptive","routing":{"conditional":{"targets":[{"model":"a"},{"model":"b"}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Conditional[0].Targets[0].When.ValueString(); got != `request.headers["x"] == "1"` {
			t.Errorf("when = %q, want the state value", got)
		}
		if !model.Conditional[0].Targets[1].When.IsNull() {
			t.Errorf("final when = %v, want null", model.Conditional[0].Targets[1].When)
		}
	})

	t.Run("a new routing mode clears the others", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Weighted: []weightedModel{{Targets: []weightedTargetModel{{Model: types.StringValue("old")}}}},
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"resilient","routing":{"failover":{"targets":[{"model":"a","priority":1}]}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if model.Weighted != nil {
			t.Errorf("weighted = %#v, want nil after switching to failover", model.Weighted)
		}
		if len(model.Failover) != 1 || len(model.Failover[0].Targets) != 1 {
			t.Fatalf("failover = %#v, want one target", model.Failover)
		}
	})

	t.Run("an unsupported routing mode preserves the model", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Name:     types.StringValue("callout"),
			Weighted: []weightedModel{{Targets: []weightedTargetModel{{Model: types.StringValue("old")}}}},
		}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":"callout","routing":{"callout":{"host":"http://127.0.0.1:8000/route"}}}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if len(model.Weighted) != 1 || model.Weighted[0].Targets[0].Model.ValueString() != "old" {
			t.Errorf("weighted = %#v, want the state value preserved", model.Weighted)
		}
	})

	t.Run("invalid JSON is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{}
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"name":`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for invalid JSON")
		}
	})
}

// TestVirtualModelSetServerMeta covers the computed attributes the harness
// stores after a write.
func TestVirtualModelSetServerMeta(t *testing.T) {
	t.Parallel()

	model := &virtualModelModel{}
	model.SetServerMeta("smart", 12)

	if got := model.StoredID.ValueString(); got != "smart" {
		t.Errorf("id = %q, want %q", got, "smart")
	}
	if got := model.Revision.ValueInt64(); got != 12 {
		t.Errorf("revision = %d, want 12", got)
	}
}
