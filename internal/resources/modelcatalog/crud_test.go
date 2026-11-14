package modelcatalog

import (
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// regexpOneAndOnlyOne matches the framework's exactly-one-of diagnostic.
var regexpOneAndOnlyOne = regexp.MustCompile(`one \(and only one\)`)

// regexpInvalidJSON matches the jsontypes diagnostic for an unparseable leaf.
var regexpInvalidJSON = regexp.MustCompile(`(?i)invalid json`)

// TestModelCatalogCRUD drives the singleton through Terraform against the
// protocol fake: create, import by the fixed id, and update from a custom
// overlay to a base catalog.
func TestModelCatalogCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkCatalogDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_model_catalog" "test" {
  custom_json = jsonencode({
    providers = {
      internal = {
        models = {
          "acme-large" = {
            rates = { input = "1.00", output = "2.00" }
          }
        }
      }
    }
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_model_catalog.test", "id", "default"),
					resource.TestCheckResourceAttr("agentgateway_model_catalog.test", "revision", "1"),
					resource.TestCheckResourceAttrSet("agentgateway_model_catalog.test", "custom_json"),
					resource.TestCheckNoResourceAttr("agentgateway_model_catalog.test", "base_json"),
				),
			},
			{
				ResourceName:      "agentgateway_model_catalog.test",
				ImportState:       true,
				ImportStateId:     "default",
				ImportStateVerify: true,
			},
			{
				Config: `
resource "agentgateway_model_catalog" "test" {
  base_json = jsonencode({
    metadata  = { generatedAt = "2026-11-14T00:00:00Z" }
    providers = {}
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_model_catalog.test", "id", "default"),
					resource.TestCheckResourceAttr("agentgateway_model_catalog.test", "revision", "2"),
					resource.TestCheckResourceAttrSet("agentgateway_model_catalog.test", "base_json"),
					resource.TestCheckNoResourceAttr("agentgateway_model_catalog.test", "custom_json"),
				),
			},
		},
	})
}

// TestModelCatalogRequiresExactlyOneLeaf pins the declarative half of the rule.
func TestModelCatalogRequiresExactlyOneLeaf(t *testing.T) {
	t.Run("no leaf", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config:      `resource "agentgateway_model_catalog" "test" {}`,
					ExpectError: regexpOneAndOnlyOne,
				},
			},
		})
	})

	t.Run("both leaves", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_model_catalog" "test" {
  base_json   = jsonencode({ providers = {} })
  custom_json = jsonencode({ providers = {} })
}`,
					ExpectError: regexpOneAndOnlyOne,
				},
			},
		})
	})
}

// TestModelCatalogRejectsInvalidJSON pins that the leaves are normalized JSON:
// the framework rejects a value that does not parse before any request.
func TestModelCatalogRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_model_catalog" "test" {
  custom_json = "{not json"
}`,
				ExpectError: regexpInvalidJSON,
			},
		},
	})
}
