package llmapikey

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// TestLLMAPIKeySetValue proves a server value round-trips into the model, that
// server-managed metadata is stripped, and that attributes the server omitted
// stay as they are instead of being zeroed (spec §7.4).
func TestLLMAPIKeySetValue(t *testing.T) {
	t.Parallel()

	t.Run("full value round-trips", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"key": "sk-acctest-0001",
			"metadata": {
				"team": "research",
				"agentgateway.dev/id": "0f0e4c0a-0000-4000-8000-000000000001",
				"agentgateway.dev/createdAt": "2026-11-14T09:00:00Z"
			},
			"allowedModels": ["gpt-4o-mini", "openai/*"],
			"budgets": [
				{
					"name": "daily-spend",
					"limit": {"unit": "USD", "amount": 50},
					"window": {"rolling": "24h"},
					"onBudgetExceeded": "Block"
				}
			],
			"createdAt": "2026-11-14T09:00:00Z",
			"updatedAt": "2026-11-14T09:30:00Z"
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Key.ValueString(); got != "sk-acctest-0001" {
			t.Errorf("key = %q, want the server value", got)
		}
		if !model.KeyHash.IsNull() {
			t.Errorf("key_hash = %v, want null", model.KeyHash)
		}

		var metadata map[string]string
		if d := model.Metadata.ElementsAs(context.Background(), &metadata, false); d.HasError() {
			t.Fatalf("decoding metadata: %v", d)
		}
		if len(metadata) != 1 || metadata["team"] != "research" {
			t.Errorf("metadata = %v, want only the user key", metadata)
		}

		var models []string
		if d := model.AllowedModels.ElementsAs(context.Background(), &models, false); d.HasError() {
			t.Fatalf("decoding allowed_models: %v", d)
		}
		if len(models) != 2 || models[0] != "gpt-4o-mini" || models[1] != "openai/*" {
			t.Errorf("allowed_models = %v, want the server list", models)
		}

		var budgets []budgetModel
		if d := model.Budgets.ElementsAs(context.Background(), &budgets, false); d.HasError() {
			t.Fatalf("decoding budgets: %v", d)
		}
		if len(budgets) != 1 {
			t.Fatalf("budgets = %v, want one entry", budgets)
		}
		budget := budgets[0]
		if got := budget.Name.ValueString(); got != "daily-spend" {
			t.Errorf("budget name = %q, want %q", got, "daily-spend")
		}
		if got := budget.OnBudgetExceeded.ValueString(); got != "Block" {
			t.Errorf("on_budget_exceeded = %q, want %q", got, "Block")
		}
		var limit budgetLimitModel
		if d := budget.Limit.As(context.Background(), &limit, basetypes.ObjectAsOptions{}); d.HasError() {
			t.Fatalf("decoding budget limit: %v", d)
		}
		if got := limit.Unit.ValueString(); got != "USD" {
			t.Errorf("limit unit = %q, want %q", got, "USD")
		}
		if got := limit.Amount.ValueFloat64(); got != 50 {
			t.Errorf("limit amount = %v, want 50", got)
		}
		var window budgetWindowModel
		if d := budget.Window.As(context.Background(), &window, basetypes.ObjectAsOptions{}); d.HasError() {
			t.Fatalf("decoding budget window: %v", d)
		}
		if got := window.Rolling.ValueString(); got != "24h" {
			t.Errorf("window rolling = %q, want %q", got, "24h")
		}

		if got := model.CreatedAt.ValueString(); got != "2026-11-14T09:00:00Z" {
			t.Errorf("created_at = %q, want the server value", got)
		}
		if got := model.UpdatedAt.ValueString(); got != "2026-11-14T09:30:00Z" {
			t.Errorf("updated_at = %q, want the server value", got)
		}
	})

	t.Run("managed metadata only reads back as unset", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"key": "sk-acctest-0002",
			"metadata": {
				"agentgateway.dev/id": "0f0e4c0a-0000-4000-8000-000000000002",
				"agentgateway.dev/createdAt": "2026-11-14T09:00:00Z"
			}
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Metadata.IsNull() {
			t.Errorf("metadata = %v, want null when only managed keys were returned", model.Metadata)
		}
		if got := model.CreatedAt.ValueString(); got != "2026-11-14T09:00:00Z" {
			t.Errorf("created_at = %q, want the managed metadata timestamp", got)
		}
	})

	t.Run("omitted attributes stay null", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"key":"sk-acctest-0003"}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.Metadata.IsNull() {
			t.Errorf("metadata = %v, want null", model.Metadata)
		}
		if !model.AllowedModels.IsNull() {
			t.Errorf("allowed_models = %v, want null", model.AllowedModels)
		}
		if !model.Budgets.IsNull() {
			t.Errorf("budgets = %v, want null", model.Budgets)
		}
		if !model.CreatedAt.IsNull() {
			t.Errorf("created_at = %v, want null", model.CreatedAt)
		}
		if !model.UpdatedAt.IsNull() {
			t.Errorf("updated_at = %v, want null", model.UpdatedAt)
		}
	})

	t.Run("omitted attributes are preserved from state", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		model.Key = types.StringValue("sk-from-state")
		model.Metadata = testMetadata(t, map[string]string{"team": "platform"})
		model.AllowedModels = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("gpt-4o")})
		model.Budgets = testBudgets(t, testBudget(t, "lifetime-tokens", "Tokens", 250, "", "Audit"))
		model.CreatedAt = types.StringValue("2026-11-01T00:00:00Z")

		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"key":"sk-from-state"}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.Metadata.Elements(); len(got) != 1 {
			t.Errorf("metadata = %v, want the state value", got)
		}
		if got := len(model.AllowedModels.Elements()); got != 1 {
			t.Errorf("allowed_models has %d elements, want the state list", got)
		}
		if got := len(model.Budgets.Elements()); got != 1 {
			t.Errorf("budgets has %d elements, want the state list", got)
		}
		if got := model.CreatedAt.ValueString(); got != "2026-11-01T00:00:00Z" {
			t.Errorf("created_at = %q, want the state value", got)
		}
	})

	t.Run("hashed key clears the plaintext key", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		model.Key = types.StringValue("sk-acctest-0004")

		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"keyHash":"`+apiKeyHash+`"}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if got := model.KeyHash.ValueString(); got != apiKeyHash {
			t.Errorf("key_hash = %q, want the server hash", got)
		}
		if !model.Key.IsNull() {
			t.Errorf("key = %v, want null so state never carries both", model.Key)
		}
	})

	t.Run("explicit null allowed models clears the constraint", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		model.AllowedModels = types.ListValueMust(types.StringType, []attr.Value{})

		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"key":"sk-acctest-0005","allowedModels":null}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}
		if !model.AllowedModels.IsNull() {
			t.Errorf("allowed_models = %v, want null", model.AllowedModels)
		}
	})

	t.Run("decimal string budget amount decodes", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"key": "sk-acctest-0006",
			"budgets": [
				{
					"name": "daily-spend",
					"limit": {"unit": "USD", "amount": "0.008"},
					"onBudgetExceeded": "Audit"
				}
			]
		}`), &diags)

		if diags.HasError() {
			t.Fatalf("SetValue() returned unexpected diagnostics: %v", diags)
		}

		var budgets []budgetModel
		if d := model.Budgets.ElementsAs(context.Background(), &budgets, false); d.HasError() {
			t.Fatalf("decoding budgets: %v", d)
		}
		if len(budgets) != 1 {
			t.Fatalf("budgets = %v, want one entry", budgets)
		}
		var limit budgetLimitModel
		if d := budgets[0].Limit.As(context.Background(), &limit, basetypes.ObjectAsOptions{}); d.HasError() {
			t.Fatalf("decoding budget limit: %v", d)
		}
		if got := limit.Amount.ValueFloat64(); got != 0.008 {
			t.Errorf("limit amount = %v, want 0.008", got)
		}
		if !budgets[0].Window.IsNull() {
			t.Errorf("window = %v, want null for a budget without a window", budgets[0].Window)
		}
	})

	t.Run("invalid JSON is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"key":`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for invalid JSON")
		}
	})

	t.Run("non-object metadata is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{"key":"sk-acctest-0007","metadata":["nope"]}`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for metadata that is not an object")
		}
	})

	t.Run("non-numeric budget amount is a diagnostic", func(t *testing.T) {
		t.Parallel()

		model := newModel()
		var diags diag.Diagnostics
		model.SetValue(context.Background(), json.RawMessage(`{
			"key": "sk-acctest-0008",
			"budgets": [
				{"name": "daily-spend", "limit": {"unit": "USD", "amount": "free"}, "onBudgetExceeded": "Block"}
			]
		}`), &diags)

		if !diags.HasError() {
			t.Fatal("SetValue() returned no error for a non-numeric budget amount")
		}
	})
}
