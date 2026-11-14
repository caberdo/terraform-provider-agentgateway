package configresources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestConfigResourcesKindAcceptsUnknownKind pins that kind is a loosely
// validated string, not an enum: a kind added by a future gateway release must
// work without a provider release.
func TestConfigResourcesKindAcceptsUnknownKind(t *testing.T) {
	t.Parallel()

	accepted := []string{
		"llm.provider",
		"traffic.gateway",
		"modelCatalog",
		"a",
		"future.kind",
		"future.nested.kind",
	}
	for _, kind := range accepted {
		resp := validator.StringResponse{}
		kindValidator().ValidateString(
			context.Background(),
			validator.StringRequest{
				ConfigValue: types.StringValue(kind),
				Path:        path.Root("kind"),
			},
			&resp,
		)
		if resp.Diagnostics.HasError() {
			t.Errorf("kind %q was rejected: %v", kind, resp.Diagnostics)
		}
	}

	rejected := []string{
		"",
		"1llm.provider",
		"llm provider",
		"llm/provider",
		".provider",
		"llm-provider",
	}
	for _, kind := range rejected {
		resp := validator.StringResponse{}
		kindValidator().ValidateString(
			context.Background(),
			validator.StringRequest{
				ConfigValue: types.StringValue(kind),
				Path:        path.Root("kind"),
			},
			&resp,
		)
		if !resp.Diagnostics.HasError() {
			t.Errorf("kind %q was accepted; the kind pattern should reject it", kind)
		}
	}
}

// TestConfigResourcesValueIsSensitive pins that value_json is sensitive: some
// kinds, llm.apiKey in particular, contain key material.
func TestConfigResourcesValueIsSensitive(t *testing.T) {
	t.Parallel()

	var resp datasource.SchemaResponse
	New().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() returned diagnostics: %v", resp.Diagnostics)
	}

	list, ok := resp.Schema.Attributes["resources"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf(`"resources" is not a list nested attribute`)
	}
	attr, ok := list.NestedObject.Attributes["value_json"]
	if !ok {
		t.Fatal(`"resources" has no "value_json" attribute`)
	}
	if !attr.IsSensitive() {
		t.Fatal(`"value_json" is not marked Sensitive; some kinds carry key material`)
	}
}

// TestConfigResourcesListsAndFilters pins the listing contract: deterministic
// order by id, the id_prefix filter and the computed "<kind>:<prefix>" id.
func TestConfigResourcesListsAndFilters(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "b" {
  name       = "cfg-b"
  value_json = jsonencode({ name = "cfg-b", provider = "openai" })
}

resource "agentgateway_test_resource" "a" {
  name       = "cfg-a"
  value_json = jsonencode({ name = "cfg-a", provider = "openai" })
}

resource "agentgateway_test_resource" "other" {
  name       = "other-provider"
  value_json = jsonencode({ name = "other-provider", provider = "openai" })
}

data "agentgateway_config_resources" "test" {
  kind      = "llm.provider"
  id_prefix = "cfg-"

  depends_on = [
    agentgateway_test_resource.a,
    agentgateway_test_resource.b,
    agentgateway_test_resource.other,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "id", "llm.provider:cfg-"),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.#", "2"),
					// Sorted by id: cfg-a before cfg-b.
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.0.id", "cfg-a"),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.0.revision", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.0.created_at", "1970-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.0.updated_at", "1970-01-01T00:00:00Z"),
					resource.TestCheckResourceAttrWith("data.agentgateway_config_resources.test", "resources.0.value_json", func(value string) error {
						var decoded map[string]any
						return json.Unmarshal([]byte(value), &decoded)
					}),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.1.id", "cfg-b"),
				),
			},
		},
	})
}

// TestConfigResourcesAcceptsUnknownKindAtRuntime proves an unknown kind is
// listed rather than rejected: the escape hatch must keep working for kinds
// this provider version has never heard of.
func TestConfigResourcesAcceptsUnknownKindAtRuntime(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
data "agentgateway_config_resources" "test" {
  kind = "future.kind"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "id", "future.kind:"),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.#", "0"),
				),
			},
		},
	})
}
