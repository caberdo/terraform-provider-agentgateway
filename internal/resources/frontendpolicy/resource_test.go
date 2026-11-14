package frontendpolicy

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

// TestFrontendPolicySchema pins the shared policy schema: an enum-checked name
// that replaces the resource when it changes, a required JSON body and the two
// computed attributes.
func TestFrontendPolicySchema(t *testing.T) {
	s := frontendPolicySchema(t)

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
		t.Fatalf("expected exactly one enum validator on name, got %d", len(name.Validators))
	}
	// "tls" is a frontend.policy key; "csrf" is a ui.policy key, so accepting
	// the first and rejecting the second proves the frontend.policy enum is
	// wired.
	assertFrontendNameEnum(t, name.Validators, "tls", "csrf")
	if len(name.PlanModifiers) != 1 {
		t.Fatalf("expected exactly one plan modifier on name (RequiresReplace), got %d", len(name.PlanModifiers))
	}
	assertFrontendNameReplaces(t, s, name.PlanModifiers, "http", "tls")

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

// TestFrontendPolicySchemaRejectsUnknownName proves the enum is wired to the
// resource: a name outside configresource.FrontendPolicyKeys never reaches the
// gateway.
func TestFrontendPolicySchemaRejectsUnknownName(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_frontend_policy" "test" {
  name = "csrf"
  spec = jsonencode({})
}`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

// TestFrontendPolicyValueIsTheSpecBodyNotWrapped asserts the request body is
// the policy body itself, never a {"spec": ...} envelope, because the gateway
// stores policy bodies verbatim.
func TestFrontendPolicyValueIsTheSpecBodyNotWrapped(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkPolicyDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_frontend_policy" "test" {
  name = "accessLog"
  spec = jsonencode({
    filter = "status >= 400"
    format = "json"
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_frontend_policy.test", "id", "accessLog"),
					resource.TestCheckResourceAttr("agentgateway_frontend_policy.test", "revision", "1"),
					func(_ *terraform.State) error {
						body := frontendPolicyBody(t, server, "accessLog")

						var got map[string]any
						if err := json.Unmarshal(body, &got); err != nil {
							return fmt.Errorf("policy body is not a JSON object: %s", body)
						}
						if _, wrapped := got["spec"]; wrapped {
							return fmt.Errorf("policy body is wrapped in a spec key: %s", body)
						}
						if got["format"] != "json" {
							return fmt.Errorf("policy body lost format: %s", body)
						}
						return nil
					},
				),
			},
			{
				// Policy bodies are stored verbatim, so import must match the
				// managed state exactly.
				ResourceName:      "agentgateway_frontend_policy.test",
				ImportState:       true,
				ImportStateId:     "accessLog",
				ImportStateVerify: true,
			},
		},
	})
}

// TestFrontendPolicyAcceptsNonObjectSpec pins that frontend.policy does not
// apply the llm.policy/apiKey body-shape rule: any JSON value is stored
// verbatim.
func TestFrontendPolicyAcceptsNonObjectSpec(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkPolicyDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_frontend_policy" "test" {
  name = "connect"
  spec = jsonencode([
    {
      host = "upstream.example.com"
      port = 8080
    }
  ])
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_frontend_policy.test", "id", "connect"),
					func(_ *terraform.State) error {
						body := frontendPolicyBody(t, server, "connect")

						var rules []map[string]any
						if err := json.Unmarshal(body, &rules); err != nil {
							return fmt.Errorf("policy body is not a JSON array: %s", body)
						}
						if len(rules) != 1 || rules[0]["host"] != "upstream.example.com" {
							return fmt.Errorf("policy body lost its fields: %s", body)
						}
						return nil
					},
				),
			},
		},
	})
}

// frontendPolicySchema reads the schema the resource itself advertises.
func frontendPolicySchema(t *testing.T) schema.Schema {
	t.Helper()

	var resp frameworkresource.SchemaResponse
	New().Schema(context.Background(), frameworkresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("reading resource schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// frontendPolicyBody returns the body of the PUT that wrote the policy with id.
func frontendPolicyBody(t *testing.T, server *agentgatewaytest.Server, id string) json.RawMessage {
	t.Helper()

	want := "/api/config/resources/" + string(configresource.KindFrontendPolicy) + "/" + id
	for _, req := range server.Requests() {
		if req.Method == http.MethodPut && req.Path == want {
			return req.Body
		}
	}
	t.Fatalf("no PUT %s was recorded", want)
	return nil
}

// assertFrontendNameEnum runs the name validator against one accepted and one
// rejected value, proving the resource wires the frontend.policy enum rather
// than another one.
func assertFrontendNameEnum(t *testing.T, validators []validator.String, accepted, rejected string) {
	t.Helper()

	for _, tc := range []struct {
		value   string
		wantErr bool
	}{
		{accepted, false},
		{rejected, true},
	} {
		var resp validator.StringResponse
		validators[0].ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("name"),
			ConfigValue: types.StringValue(tc.value),
		}, &resp)
		if resp.Diagnostics.HasError() != tc.wantErr {
			t.Errorf("name = %q: diagnostics = %v, wantErr %v", tc.value, resp.Diagnostics, tc.wantErr)
		}
	}
}

// assertFrontendNameReplaces proves a changed name forces replacement: a policy
// cannot be renamed because its name is its id (spec §7.0).
func assertFrontendNameReplaces(t *testing.T, s schema.Schema, modifiers []planmodifier.String, oldName, newName string) {
	t.Helper()

	var resp planmodifier.StringResponse
	modifiers[0].PlanModifyString(context.Background(), planmodifier.StringRequest{
		State:      tfsdk.State{Schema: s, Raw: frontendPolicyRaw(oldName)},
		Plan:       tfsdk.Plan{Schema: s, Raw: frontendPolicyRaw(newName)},
		StateValue: types.StringValue(oldName),
		PlanValue:  types.StringValue(newName),
		Path:       path.Root("name"),
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("plan modifier diagnostics: %v", resp.Diagnostics)
	}
	if !resp.RequiresReplace {
		t.Errorf("changing name from %q to %q did not require replacement", oldName, newName)
	}
}

// frontendPolicyRaw is a complete policy object with the given name, used to
// give the plan modifier a real state and plan instead of the null values that
// mean create and destroy.
func frontendPolicyRaw(name string) tftypes.Value {
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
