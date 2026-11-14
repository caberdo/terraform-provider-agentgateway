package modelcatalog

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// TestModelCatalogValue pins the value object of agentgateway_model_catalog:
// at least one JSON leaf, inserted verbatim under base and/or custom
// (spec §7.15).
func TestModelCatalogValue(t *testing.T) {
	tests := []struct {
		name    string
		model   modelCatalogModel
		want    string
		wantErr string
	}{
		{
			name: "base catalog",
			model: modelCatalogModel{
				BaseJSON:   jsontypes.NewNormalizedValue(`{"metadata":{"generatedAt":"2026-11-14T00:00:00Z"},"providers":{}}`),
				CustomJSON: jsontypes.NewNormalizedNull(),
			},
			want: `{"base": {"metadata": {"generatedAt": "2026-11-14T00:00:00Z"}, "providers": {}}}`,
		},
		{
			name: "custom overlay",
			model: modelCatalogModel{
				BaseJSON: jsontypes.NewNormalizedNull(),
				CustomJSON: jsontypes.NewNormalizedValue(
					`{"providers":{"openai":{"models":{"gpt-4o":{"rates":{"input":"2.5"}}}}}}`,
				),
			},
			want: `{"custom": {"providers": {"openai": {"models": {"gpt-4o": {"rates": {"input": "2.5"}}}}}}}`,
		},
		{
			name: "custom overlay is not required to be an object",
			model: modelCatalogModel{
				CustomJSON: jsontypes.NewNormalizedValue(`["free-model"]`),
			},
			want: `{"custom": ["free-model"]}`,
		},
		{
			name:    "no leaf",
			model:   modelCatalogModel{},
			wantErr: "at least one of base_json or custom_json must be set",
		},
		{
			name: "base catalog with an overlay",
			model: modelCatalogModel{
				BaseJSON:   jsontypes.NewNormalizedValue(`{"metadata":{"generatedAt":"2026-11-14T00:00:00Z"},"providers":{}}`),
				CustomJSON: jsontypes.NewNormalizedValue(`{"providers":{"internal":{}}}`),
			},
			want: `{
				"base": {"metadata": {"generatedAt": "2026-11-14T00:00:00Z"}, "providers": {}},
				"custom": {"providers": {"internal": {}}}
			}`,
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

// TestModelCatalogID pins the singleton id.
func TestModelCatalogID(t *testing.T) {
	t.Parallel()

	model := newModel()
	if got := model.ID(); got != "default" {
		t.Errorf("ID() = %q, want %q", got, "default")
	}

	model.SetServerMeta("default", 4)
	if got := model.IDValue.ValueString(); got != "default" {
		t.Errorf("id = %q, want %q", got, "default")
	}
	if got := model.Revision.ValueInt64(); got != 4 {
		t.Errorf("revision = %d, want 4", got)
	}
}

// TestModelCatalogSchemaRequiresAtLeastOneLeaf pins the declarative half of the
// rule: both leaves are optional, normalized JSON, and either may be omitted.
func TestModelCatalogSchemaRequiresAtLeastOneLeaf(t *testing.T) {
	t.Parallel()

	catalogSchema := newSchema()
	for _, name := range []string{"base_json", "custom_json"} {
		attribute, ok := catalogSchema.Attributes[name]
		if !ok {
			t.Fatalf("%s is missing from the schema", name)
		}
		if !attribute.IsOptional() {
			t.Errorf("%s is not optional", name)
		}
		if attribute.GetType() != (jsontypes.NormalizedType{}) {
			t.Errorf("%s type = %T, want jsontypes.Normalized", name, attribute.GetType())
		}
	}

	if !catalogSchema.Attributes["id"].IsComputed() {
		t.Error("id is not computed")
	}
	if !catalogSchema.Attributes["revision"].IsComputed() {
		t.Error("revision is not computed")
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
