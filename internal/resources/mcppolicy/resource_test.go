package mcppolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestMCPPolicySchema pins the shared policy schema: an enum-checked name that
// replaces the resource when it changes, a required JSON body and the two
// computed attributes.
func TestMCPPolicySchema(t *testing.T) {
	s := resourceSchema(t)

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
	assertNameEnum(t, name.Validators, "mcpAuthentication", "guardrails")
	assertNameReplaces(t, s, name.PlanModifiers, "cors", "mcpAuthentication")

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

	for _, computed := range []string{"id", "revision"} {
		attribute, ok := s.Attributes[computed]
		if !ok {
			t.Fatalf("%s attribute is missing", computed)
		}
		if !attribute.IsComputed() || attribute.IsRequired() || attribute.IsOptional() {
			t.Errorf("%s must be computed only", computed)
		}
	}
}

// TestMCPPolicySchemaRejectsLLMOnlyName proves the MCP enum is wired to the
// resource rather than the LLM one: guardrails exists only for llm.policy.
func TestMCPPolicySchemaRejectsLLMOnlyName(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_policy" "test" {
  name = "guardrails"
  spec = jsonencode({})
}`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

// TestPolicyValueIsTheSpecObjectNotWrapped asserts the request body is the
// policy body itself, never a {"spec": ...} envelope, because the gateway
// stores policy bodies verbatim.
func TestPolicyValueIsTheSpecObjectNotWrapped(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkPolicyDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_policy" "test" {
  name = "cors"
  spec = jsonencode({
    allowOrigins = ["https://example.com"]
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_policy.test", "id", "cors"),
					resource.TestCheckResourceAttr("agentgateway_mcp_policy.test", "revision", "1"),
					func(_ *terraform.State) error {
						body := policyBody(t, server, "cors")

						var got map[string]any
						if err := json.Unmarshal(body, &got); err != nil {
							return fmt.Errorf("policy body is not a JSON object: %s", body)
						}
						if _, wrapped := got["spec"]; wrapped {
							return fmt.Errorf("policy body is wrapped in a spec key: %s", body)
						}
						origins, ok := got["allowOrigins"].([]any)
						if !ok || len(origins) != 1 || origins[0] != "https://example.com" {
							return fmt.Errorf("policy body lost allowOrigins: %s", body)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "agentgateway_mcp_policy.test",
				ImportState:       true,
				ImportStateId:     "cors",
				ImportStateVerify: true,
			},
		},
	})
}

// TestMCPPolicyAcceptsArraySpec covers localRateLimit, whose documented body is
// an array of rate-limit rules rather than an object.
func TestMCPPolicyAcceptsArraySpec(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkPolicyDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_policy" "test" {
  name = "localRateLimit"
  spec = jsonencode([
    {
      requestsPerUnit = 5
      unit            = "minute"
    }
  ])
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_policy.test", "id", "localRateLimit"),
					func(_ *terraform.State) error {
						body := policyBody(t, server, "localRateLimit")

						var rules []map[string]any
						if err := json.Unmarshal(body, &rules); err != nil {
							return fmt.Errorf("policy body is not a JSON array: %s", body)
						}
						if len(rules) != 1 || rules[0]["requestsPerUnit"] != float64(5) {
							return fmt.Errorf("rate-limit rules lost their fields: %s", body)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      "agentgateway_mcp_policy.test",
				ImportState:       true,
				ImportStateId:     "localRateLimit",
				ImportStateVerify: true,
			},
		},
	})
}

// TestMCPPolicyAPIKeyAcceptsInlineKeys documents that the C11 keys ban is
// specific to llm.policy: an mcp.policy/apiKey body may carry keys.
func TestMCPPolicyAPIKeyAcceptsInlineKeys(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkPolicyDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_policy" "test" {
  name = "apiKey"
  spec = jsonencode({
    keys = ["secret"]
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_policy.test", "id", "apiKey"),
					func(_ *terraform.State) error {
						body := policyBody(t, server, "apiKey")

						var got map[string]any
						if err := json.Unmarshal(body, &got); err != nil {
							return fmt.Errorf("policy body is not a JSON object: %s", body)
						}
						keys, ok := got["keys"].([]any)
						if !ok || len(keys) != 1 {
							return fmt.Errorf("policy body lost keys: %s", body)
						}
						return nil
					},
				),
			},
		},
	})
}

// resourceSchema reads the schema the resource itself advertises.
func resourceSchema(t *testing.T) schema.Schema {
	t.Helper()

	var resp frameworkresource.SchemaResponse
	New().Schema(context.Background(), frameworkresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("reading resource schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// policyBody returns the body of the PUT that wrote the policy with id.
func policyBody(t *testing.T, server *agentgatewaytest.Server, id string) json.RawMessage {
	t.Helper()

	want := "/api/config/resources/" + string(configresource.KindMCPPolicy) + "/" + id
	for _, req := range server.Requests() {
		if req.Method == http.MethodPut && req.Path == want {
			return req.Body
		}
	}
	t.Fatalf("no PUT %s was recorded", want)
	return nil
}

// assertNameEnum asserts that the name validators accept one enum value and
// reject another. It checks containment rather than cardinality, so an extra
// shared validator does not break this test.
func assertNameEnum(t *testing.T, validators []validator.String, accepted, rejected string) {
	t.Helper()

	if len(validators) == 0 {
		t.Fatal("name has no validators; expected an enum validator")
	}
	if detail := validateName(validators, accepted); detail != "" {
		t.Errorf("name = %q was rejected: %s", accepted, detail)
	}
	if detail := validateName(validators, rejected); detail == "" {
		t.Errorf("name = %q was accepted; expected the enum validator to reject it", rejected)
	}
}

// validateName runs every validator and returns the first diagnostic detail, or
// "" when they all accept the value.
func validateName(validators []validator.String, value string) string {
	for _, v := range validators {
		var resp validator.StringResponse
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("name"),
			ConfigValue: types.StringValue(value),
		}, &resp)
		if resp.Diagnostics.HasError() {
			return resp.Diagnostics.Errors()[0].Detail()
		}
	}
	return ""
}

// assertNameReplaces proves a changed name forces replacement: a policy cannot
// be renamed because its name is its id (spec §7.0). It checks containment
// rather than cardinality, so an extra plan modifier does not break this test.
func assertNameReplaces(t *testing.T, s schema.Schema, modifiers []planmodifier.String, oldName, newName string) {
	t.Helper()

	if len(modifiers) == 0 {
		t.Fatal("name has no plan modifiers; expected RequiresReplace")
	}
	for _, m := range modifiers {
		var resp planmodifier.StringResponse
		m.PlanModifyString(context.Background(), planmodifier.StringRequest{
			State:      tfsdk.State{Schema: s, Raw: policyRaw(oldName)},
			Plan:       tfsdk.Plan{Schema: s, Raw: policyRaw(newName)},
			StateValue: types.StringValue(oldName),
			PlanValue:  types.StringValue(newName),
			Path:       path.Root("name"),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("plan modifier diagnostics: %v", resp.Diagnostics)
		}
		if resp.RequiresReplace {
			return
		}
	}
	t.Errorf("changing name from %q to %q did not require replacement", oldName, newName)
}

// policyRaw is a complete policy object with the given name, used to give the
// plan modifier a real state and plan instead of the null values that mean
// create and destroy.
func policyRaw(name string) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"name":     tftypes.String,
		"spec":     tftypes.String,
		"id":       tftypes.String,
		"revision": tftypes.Number,
	}}, map[string]tftypes.Value{
		"name":     tftypes.NewValue(tftypes.String, name),
		"spec":     tftypes.NewValue(tftypes.String, `{}`),
		"id":       tftypes.NewValue(tftypes.String, name),
		"revision": tftypes.NewValue(tftypes.Number, 1),
	})
}
