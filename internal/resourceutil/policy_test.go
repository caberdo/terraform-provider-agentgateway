package resourceutil_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const apiKeyKeysDiagnostic = "llm.policy/apiKey must not include keys; use agentgateway_llm_api_key resources"

func policyAttrTypes() map[string]tftypes.Type {
	return map[string]tftypes.Type{
		"name":     tftypes.String,
		"spec":     tftypes.String,
		"id":       tftypes.String,
		"revision": tftypes.Number,
	}
}

func newPolicyModelFromPlan(t *testing.T, kind configresource.Kind, name, spec string) resourceutil.KindResource {
	t.Helper()

	s := resourceutil.PolicySchema(configresource.LLMPolicyKeys)
	plan := tfsdk.Plan{
		Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: policyAttrTypes()}, map[string]tftypes.Value{
			"name":     tftypes.NewValue(tftypes.String, name),
			"spec":     tftypes.NewValue(tftypes.String, spec),
			"id":       tftypes.NewValue(tftypes.String, nil),
			"revision": tftypes.NewValue(tftypes.Number, nil),
		}),
		Schema: s,
	}

	model := resourceutil.NewPolicyModel(kind)
	if model == nil {
		t.Fatal("NewPolicyModel returned nil")
	}

	var diags diag.Diagnostics
	diags.Append(plan.Get(context.Background(), model)...)
	if diags.HasError() {
		t.Fatalf("building policy model: %v", diags)
	}
	return model
}

func TestPolicySchemaRequiresExactlyOneBody(t *testing.T) {
	s := resourceutil.PolicySchema([]string{"cors", "apiKey"})

	if len(s.Attributes) != 4 {
		t.Fatalf("expected 4 attributes (name, spec, id, revision), got %d", len(s.Attributes))
	}

	name, ok := s.Attributes["name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("name attribute is %T, want schema.StringAttribute", s.Attributes["name"])
	}
	if !name.Required || name.Optional || name.Computed {
		t.Errorf("name must be required-only, got required=%v optional=%v computed=%v", name.Required, name.Optional, name.Computed)
	}
	if len(name.Validators) != 1 {
		t.Errorf("expected one enum validator on name, got %d", len(name.Validators))
	}
	if len(name.PlanModifiers) != 1 {
		t.Errorf("expected one plan modifier on name (RequiresReplace), got %d", len(name.PlanModifiers))
	}

	spec, ok := s.Attributes["spec"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("spec attribute is %T, want schema.StringAttribute", s.Attributes["spec"])
	}
	if !spec.Required || spec.Optional || spec.Computed {
		t.Errorf("spec must be required-only, got required=%v optional=%v computed=%v", spec.Required, spec.Optional, spec.Computed)
	}
	if _, ok := spec.CustomType.(jsontypes.NormalizedType); !ok {
		t.Errorf("spec custom type is %T, want jsontypes.NormalizedType", spec.CustomType)
	}

	bodyAttributes := 0
	for attributeName, attribute := range s.Attributes {
		if attribute.IsRequired() && attributeName != "name" {
			bodyAttributes++
			if attributeName != "spec" {
				t.Errorf("unexpected required body attribute %q", attributeName)
			}
		}
	}
	if bodyAttributes != 1 {
		t.Errorf("expected exactly one required body attribute, got %d", bodyAttributes)
	}

	for _, computed := range []string{"id", "revision"} {
		attribute := s.Attributes[computed]
		if attribute == nil {
			t.Fatalf("%s attribute is missing", computed)
		}
		if !attribute.IsComputed() {
			t.Errorf("%s must be computed", computed)
		}
	}
}

func TestPolicyModelRejectsKeysInAPIKeySpec(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"object with keys", `{"keys":["secret"]}`, true},
		{"array body", `[{"key":"secret"}]`, true},
		{"empty object", `{}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := newPolicyModelFromPlan(t, configresource.KindLLMPolicy, "apiKey", tc.body)

			var diags diag.Diagnostics
			value := model.Value(context.Background(), &diags)

			if diags.HasError() != tc.wantErr {
				t.Fatalf("Value() diagnostics = %v, wantErr %v", diags, tc.wantErr)
			}
			if !tc.wantErr {
				if string(value) != tc.body {
					t.Errorf("expected spec returned verbatim %s, got %s", tc.body, value)
				}
				return
			}
			detail := diags.Errors()[0].Detail()
			if !strings.Contains(detail, apiKeyKeysDiagnostic) {
				t.Errorf("expected %q in diagnostic, got %q", apiKeyKeysDiagnostic, detail)
			}
		})
	}
}

func TestPolicyModelAcceptsAnyJSONBody(t *testing.T) {
	body := `[{"requestsPerUnit":10}]`
	model := newPolicyModelFromPlan(t, configresource.KindLLMPolicy, "localRateLimit", body)

	var diags diag.Diagnostics
	value := model.Value(context.Background(), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if string(value) != body {
		t.Errorf("expected array spec returned verbatim, got %s", value)
	}

	// The apiKey rule is specific to llm.policy; other policy kinds are not
	// subject to it.
	mcpModel := newPolicyModelFromPlan(t, configresource.KindMCPPolicy, "apiKey", `{"keys":[]}`)
	diags = diag.Diagnostics{}
	mcpModel.Value(context.Background(), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected error for mcp.policy: %v", diags)
	}
}

func TestPolicyModelIDAndServerMeta(t *testing.T) {
	model := newPolicyModelFromPlan(t, configresource.KindLLMPolicy, "cors", `{}`)
	if got := model.ID(); got != "cors" {
		t.Errorf("ID() = %q, want cors", got)
	}

	model.SetServerMeta("cors", 4)
	if got := model.ID(); got != "cors" {
		t.Errorf("ID() after SetServerMeta = %q, want cors", got)
	}

	// An import has no name in state; SetServerMeta must seed it from the id so
	// the required name attribute is never left null.
	imported := resourceutil.NewPolicyModel(configresource.KindLLMPolicy)
	imported.SetServerMeta("jwtAuth", 0)
	if got := imported.ID(); got != "jwtAuth" {
		t.Errorf("imported ID() = %q, want jwtAuth", got)
	}
}

func TestPolicyModelKindsAndSetValue(t *testing.T) {
	kinds := []configresource.Kind{
		configresource.KindLLMPolicy,
		configresource.KindMCPPolicy,
		configresource.KindUIPolicy,
		configresource.KindFrontendPolicy,
	}
	for _, kind := range kinds {
		model := resourceutil.NewPolicyModel(kind)
		if model == nil {
			t.Fatalf("NewPolicyModel(%s) returned nil", kind)
		}
		if model.Kind() != kind {
			t.Errorf("Kind() = %s, want %s", model.Kind(), kind)
		}

		var kindDiags diag.Diagnostics
		if value := model.Value(context.Background(), &kindDiags); value != nil || !kindDiags.HasError() {
			t.Errorf("%s: expected a diagnostic for a null spec, got value=%s", kind, value)
		}

		kindDiags = diag.Diagnostics{}
		model.SetValue(context.Background(), json.RawMessage(`{}`), &kindDiags)
		if value := model.Value(context.Background(), &kindDiags); string(value) != `{}` || kindDiags.HasError() {
			t.Errorf("%s: expected the spec round-tripped, got %s (%v)", kind, value, kindDiags)
		}
	}

	if model := resourceutil.NewPolicyModel(configresource.KindLLMProvider); model != nil {
		t.Errorf("expected nil for a non-policy kind, got %T", model)
	}

	model := resourceutil.NewPolicyModel(configresource.KindLLMPolicy)
	var diags diag.Diagnostics
	if value := model.Value(context.Background(), &diags); value != nil || !diags.HasError() {
		t.Fatalf("expected a diagnostic for a null spec, got value=%s diags=%v", value, diags)
	}

	diags = diag.Diagnostics{}
	model.SetValue(context.Background(), json.RawMessage(`{"a":1}`), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected SetValue error: %v", diags)
	}
	value := model.Value(context.Background(), &diags)
	if string(value) != `{"a":1}` {
		t.Errorf("expected the server value round-tripped, got %s", value)
	}

	// An empty value (for example a file-mode response) leaves the spec alone.
	before := string(value)
	model.SetValue(context.Background(), nil, &diags)
	if got := model.Value(context.Background(), &diags); string(got) != before {
		t.Errorf("empty SetValue must preserve spec, got %s", got)
	}
}
