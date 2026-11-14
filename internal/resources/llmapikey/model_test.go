package llmapikey

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// apiKeyHash is a syntactically valid sha256 key hash used by the tests. It is
// the hash of the test key, never a real credential.
const apiKeyHash = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

// TestLLMAPIKeyValue pins the value object of agentgateway_llm_api_key: the
// key/keyHash union, metadata, and the null-versus-empty distinction of
// allowedModels (spec C12, C13, §7.4).
func TestLLMAPIKeyValue(t *testing.T) {
	tests := []struct {
		name    string
		model   apiKeyModel
		want    string
		wantErr string
	}{
		{
			name:  "plaintext key",
			model: apiKeyModel{Key: types.StringValue("sk-acctest-0001")},
			want:  `{"key": "sk-acctest-0001"}`,
		},
		{
			name:  "hashed key",
			model: apiKeyModel{KeyHash: types.StringValue(apiKeyHash)},
			want:  `{"keyHash": "` + apiKeyHash + `"}`,
		},
		{
			name: "both key and key_hash",
			model: apiKeyModel{
				Key:     types.StringValue("sk-acctest-0001"),
				KeyHash: types.StringValue(apiKeyHash),
			},
			wantErr: "only one of key or key_hash may be set",
		},
		{
			name:    "neither key nor key_hash",
			model:   apiKeyModel{Metadata: types.MapNull(types.StringType)},
			wantErr: "exactly one of key or key_hash must be set",
		},
		{
			name: "metadata with allowed_models unset is unconstrained",
			model: apiKeyModel{
				Key:           types.StringValue("sk-acctest-0002"),
				Metadata:      testMetadata(t, map[string]string{"team": "research"}),
				AllowedModels: types.ListNull(types.StringType),
			},
			want: `{
				"key": "sk-acctest-0002",
				"metadata": {"team": "research"}
			}`,
		},
		{
			name: "empty allowed_models denies every model",
			model: apiKeyModel{
				Key:           types.StringValue("sk-acctest-0003"),
				AllowedModels: types.ListValueMust(types.StringType, []attr.Value{}),
			},
			want: `{"key": "sk-acctest-0003", "allowedModels": []}`,
		},
		{
			name: "allowed_models patterns",
			model: apiKeyModel{
				Key: types.StringValue("sk-acctest-0004"),
				AllowedModels: types.ListValueMust(types.StringType, []attr.Value{
					types.StringValue("gpt-4o-mini"),
					types.StringValue("openai/*"),
				}),
			},
			want: `{"key": "sk-acctest-0004", "allowedModels": ["gpt-4o-mini", "openai/*"]}`,
		},
		{
			name: "budget with a rolling window",
			model: apiKeyModel{
				Key:           types.StringValue("sk-acctest-0005"),
				AllowedModels: types.ListNull(types.StringType),
				Budgets: testBudgets(t,
					testBudget(t, "daily-spend", "USD", 50, "24h", "Block"),
				),
			},
			want: `{
				"key": "sk-acctest-0005",
				"budgets": [
					{
						"name": "daily-spend",
						"limit": {"unit": "USD", "amount": 50},
						"window": {"rolling": "24h"},
						"onBudgetExceeded": "Block"
					}
				]
			}`,
		},
		{
			name: "budget without a window never resets",
			model: apiKeyModel{
				Key: types.StringValue("sk-acctest-0006"),
				Budgets: testBudgets(t,
					testBudget(t, "lifetime-tokens", "Tokens", 250, "", "Audit"),
				),
			},
			want: `{
				"key": "sk-acctest-0006",
				"budgets": [
					{
						"name": "lifetime-tokens",
						"limit": {"unit": "Tokens", "amount": 250},
						"onBudgetExceeded": "Audit"
					}
				]
			}`,
		},
		{
			name: "reserved metadata prefix is rejected",
			model: apiKeyModel{
				Key:      types.StringValue("sk-acctest-0007"),
				Metadata: testMetadata(t, map[string]string{"agentgateway.dev/team": "research"}),
			},
			wantErr: `uses reserved prefix "agentgateway.dev/"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var diags diag.Diagnostics
			got := tt.model.Value(context.Background(), &diags)

			if tt.wantErr != "" {
				if !diags.HasError() {
					t.Fatalf("Value() returned no error, want %q", tt.wantErr)
				}
				if detail := diags[0].Detail(); !strings.Contains(detail, tt.wantErr) {
					t.Fatalf("Value() detail = %q, want it to contain %q", detail, tt.wantErr)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("Value() returned unexpected diagnostics: %v", diags)
			}
			assertJSONEqual(t, got, tt.want)
		})
	}
}

// TestLLMAPIKeyID covers the computed id: the server generates a UUID, so the
// model reports whatever the server stored instead of deriving an id.
func TestLLMAPIKeyID(t *testing.T) {
	t.Parallel()

	model := newModel()
	if got := model.ID(); got != "" {
		t.Errorf("ID() = %q before the server assigned one, want empty", got)
	}

	model.SetServerMeta("0f0e4c0a-0000-4000-8000-000000000001", 3)
	if got := model.ID(); got != "0f0e4c0a-0000-4000-8000-000000000001" {
		t.Errorf("ID() = %q, want the server id", got)
	}
	if got := model.Revision.ValueInt64(); got != 3 {
		t.Errorf("revision = %d, want 3", got)
	}
}

func testMetadata(t *testing.T, metadata map[string]string) types.Map {
	t.Helper()

	value, diags := types.MapValueFrom(context.Background(), types.StringType, metadata)
	if diags.HasError() {
		t.Fatalf("building metadata map: %v", diags)
	}
	return value
}

func testBudgets(t *testing.T, budgets ...budgetModel) types.List {
	t.Helper()

	value, diags := types.ListValueFrom(context.Background(), budgetObjectType(), budgets)
	if diags.HasError() {
		t.Fatalf("building budgets list: %v", diags)
	}
	return value
}

func testBudget(t *testing.T, name, unit string, amount float64, rolling, action string) budgetModel {
	t.Helper()

	limit, diags := types.ObjectValue(budgetLimitAttrTypes(), map[string]attr.Value{
		"unit":   types.StringValue(unit),
		"amount": types.Float64Value(amount),
	})
	if diags.HasError() {
		t.Fatalf("building budget limit: %v", diags)
	}

	window := types.ObjectNull(budgetWindowAttrTypes())
	if rolling != "" {
		value, windowDiags := types.ObjectValue(budgetWindowAttrTypes(), map[string]attr.Value{
			"rolling": types.StringValue(rolling),
		})
		if windowDiags.HasError() {
			t.Fatalf("building budget window: %v", windowDiags)
		}
		window = value
	}

	return budgetModel{
		Name:             types.StringValue(name),
		Limit:            limit,
		Window:           window,
		OnBudgetExceeded: types.StringValue(action),
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
