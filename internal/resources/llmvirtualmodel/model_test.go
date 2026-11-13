package llmvirtualmodel

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestVirtualModelValue pins the API value object for every routing mode.
func TestVirtualModelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// model builds the Terraform-side model.
		model func() *virtualModelModel
		// want is the exact JSON value object sent to the API.
		want string
		// wantErr is a fragment that the error diagnostics must contain.
		wantErr string
	}{
		{
			name: "weighted with explicit weights",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name: types.StringValue("smart"),
					Weighted: []weightedModel{{
						Targets: []weightedTargetModel{
							{Model: types.StringValue("gpt-4o-primary"), Weight: types.Int64Value(90)},
							{Model: types.StringValue("gpt-4o-fallback"), Weight: types.Int64Value(10)},
						},
					}},
				}
			},
			want: `{"name":"smart","routing":{"weighted":{"targets":[{"model":"gpt-4o-primary","weight":90},{"model":"gpt-4o-fallback","weight":10}]}}}`,
		},
		{
			name: "weighted omits a null weight so the server default applies",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name: types.StringValue("smart"),
					Weighted: []weightedModel{{
						Targets: []weightedTargetModel{
							{Model: types.StringValue("gpt-4o-primary")},
							{Model: types.StringValue("gpt-4o-fallback"), Weight: types.Int64Value(1)},
						},
					}},
				}
			},
			want: `{"name":"smart","routing":{"weighted":{"targets":[{"model":"gpt-4o-primary"},{"model":"gpt-4o-fallback","weight":1}]}}}`,
		},
		{
			name: "failover with priorities",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name: types.StringValue("resilient"),
					Failover: []failoverModel{{
						Targets: []failoverTargetModel{
							{Model: types.StringValue("claude-primary"), Priority: types.Int64Value(0)},
							{Model: types.StringValue("claude-backup"), Priority: types.Int64Value(1)},
						},
					}},
				}
			},
			want: `{"name":"resilient","routing":{"failover":{"targets":[{"model":"claude-primary","priority":0},{"model":"claude-backup","priority":1}]}}}`,
		},
		{
			name: "failover omits a null priority",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name: types.StringValue("resilient"),
					Failover: []failoverModel{{
						Targets: []failoverTargetModel{
							{Model: types.StringValue("claude-primary")},
						},
					}},
				}
			},
			want: `{"name":"resilient","routing":{"failover":{"targets":[{"model":"claude-primary"}]}}}`,
		},
		{
			name: "conditional with when expressions",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name: types.StringValue("adaptive"),
					Conditional: []conditionalModel{{
						Targets: []conditionalTargetModel{
							{Model: types.StringValue("openai-fast"), When: types.StringValue(`request.headers["x-tier"] == "free"`)},
							{Model: types.StringValue("openai-smart"), When: types.StringValue(`request.headers["x-tier"] == "pro"`)},
						},
					}},
				}
			},
			want: `{"name":"adaptive","routing":{"conditional":{"targets":[{"model":"openai-fast","when":"request.headers[\"x-tier\"] == \"free\""},{"model":"openai-smart","when":"request.headers[\"x-tier\"] == \"pro\""}]}}}`,
		},
		{
			name: "conditional omits when on the final target",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name: types.StringValue("adaptive"),
					Conditional: []conditionalModel{{
						Targets: []conditionalTargetModel{
							{Model: types.StringValue("openai-fast"), When: types.StringValue(`request.headers["x-tier"] == "free"`)},
							{Model: types.StringValue("openai-smart")},
						},
					}},
				}
			},
			want: `{"name":"adaptive","routing":{"conditional":{"targets":[{"model":"openai-fast","when":"request.headers[\"x-tier\"] == \"free\""},{"model":"openai-smart"}]}}}`,
		},
		{
			name: "zero routing blocks",
			model: func() *virtualModelModel {
				return &virtualModelModel{Name: types.StringValue("broken")}
			},
			wantErr: "exactly one of weighted, failover or conditional",
		},
		{
			name: "two routing blocks",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name:     types.StringValue("broken"),
					Weighted: []weightedModel{{Targets: []weightedTargetModel{{Model: types.StringValue("a")}}}},
					Failover: []failoverModel{{Targets: []failoverTargetModel{{Model: types.StringValue("b")}}}},
				}
			},
			wantErr: "exactly one of weighted, failover or conditional",
		},
		{
			name: "routing block without targets",
			model: func() *virtualModelModel {
				return &virtualModelModel{
					Name:     types.StringValue("broken"),
					Weighted: []weightedModel{{}},
				}
			},
			wantErr: "at least one target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var diags diag.Diagnostics
			got := tt.model().Value(context.Background(), &diags)

			if tt.wantErr != "" {
				if !diags.HasError() {
					t.Fatalf("Value() returned no error, want one containing %q", tt.wantErr)
				}
				if !diagnosticsContain(diags, tt.wantErr) {
					t.Fatalf("Value() diagnostics %v do not contain %q", diags, tt.wantErr)
				}
				return
			}

			if diags.HasError() {
				t.Fatalf("Value() returned unexpected diagnostics: %v", diags)
			}
			if string(got) != tt.want {
				t.Errorf("Value() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestVirtualModelRoutingIsExclusive covers the belt-and-braces routing rules:
// exactly one block, and a conditional target may omit when only when it is
// the final target.
func TestVirtualModelRoutingIsExclusive(t *testing.T) {
	t.Parallel()

	t.Run("two routing blocks are rejected", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Name:        types.StringValue("broken"),
			Weighted:    []weightedModel{{Targets: []weightedTargetModel{{Model: types.StringValue("a")}}}},
			Conditional: []conditionalModel{{Targets: []conditionalTargetModel{{Model: types.StringValue("b")}}}},
		}

		var diags diag.Diagnostics
		model.Value(context.Background(), &diags)

		if !diags.HasError() {
			t.Fatal("Value() returned no error for two routing blocks")
		}
		if !diagnosticsContain(diags, "exactly one of weighted, failover or conditional") {
			t.Fatalf("diagnostics %v do not explain the exclusivity rule", diags)
		}
	})

	t.Run("zero routing blocks are rejected", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{Name: types.StringValue("broken")}

		var diags diag.Diagnostics
		model.Value(context.Background(), &diags)

		if !diags.HasError() {
			t.Fatal("Value() returned no error for zero routing blocks")
		}
		if !diagnosticsContain(diags, "exactly one of weighted, failover or conditional") {
			t.Fatalf("diagnostics %v do not explain the exclusivity rule", diags)
		}
	})

	t.Run("conditional when may be omitted only on the last target", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Name: types.StringValue("adaptive"),
			Conditional: []conditionalModel{{
				Targets: []conditionalTargetModel{
					{Model: types.StringValue("first")},
					{Model: types.StringValue("last")},
				},
			}},
		}

		var diags diag.Diagnostics
		model.Value(context.Background(), &diags)

		if !diags.HasError() {
			t.Fatal("Value() returned no error for a non-final target without when")
		}
		if !diagnosticsContain(diags, "target 0") {
			t.Fatalf("diagnostics %v do not name the offending target index", diags)
		}
	})

	t.Run("conditional last target without when is accepted", func(t *testing.T) {
		t.Parallel()

		model := &virtualModelModel{
			Name: types.StringValue("adaptive"),
			Conditional: []conditionalModel{{
				Targets: []conditionalTargetModel{
					{Model: types.StringValue("first"), When: types.StringValue("true")},
					{Model: types.StringValue("last")},
				},
			}},
		}

		var diags diag.Diagnostics
		model.Value(context.Background(), &diags)

		if diags.HasError() {
			t.Fatalf("Value() returned unexpected diagnostics: %v", diags)
		}
	})
}

// diagnosticsContain reports whether any diagnostic detail or summary contains
// the fragment.
func diagnosticsContain(diags diag.Diagnostics, fragment string) bool {
	for _, d := range diags {
		if strings.Contains(d.Summary(), fragment) || strings.Contains(d.Detail(), fragment) {
			return true
		}
	}
	return false
}
