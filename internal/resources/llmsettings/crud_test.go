package llmsettings

import (
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLLMSettingsCRUD drives the singleton through Terraform against the
// protocol fake: create, import and update from gateways to the deprecated
// port listener.
func TestLLMSettingsCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkSettingsDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_settings" "test" {
  gateways = ["default", "llm/listener"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "id", "default"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "revision", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.0", "default"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.1", "llm/listener"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_settings.test", "port"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_settings.test",
				ImportState:       true,
				ImportStateId:     "default",
				ImportStateVerify: true,
			},
			{
				Config: `
resource "agentgateway_llm_settings" "test" {
  port = 8080

  tls_json = jsonencode({
    cert = "/etc/tls/tls.crt"
    key  = "/etc/tls/tls.key"
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "port", "8080"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_settings.test", "gateways.#"),
				),
			},
		},
	})
}

// TestLLMSettingsSchemaConflicts pins that a populated gateways list and port
// cannot be set together, and that an empty list is accepted next to port
// (which is what the gateway UI writes when no gateway is bound).
func TestLLMSettingsSchemaConflicts(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_settings" "test" {
  gateways = ["default"]
  port     = 8080
}`,
				ExpectError: regexp.MustCompile(`gateways and port conflict`),
			},
			{
				Config: `
resource "agentgateway_llm_settings" "test" {
  gateways = []
  port     = 8080
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "port", "8080"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.#", "0"),
				),
			},
		},
	})
}

// TestLLMSettingsRejectedInFileMode pins the singleton's storage-mode rule:
// file mode cannot read the overlay back, so the resource refuses up front
// instead of dropping itself from state on the next refresh.
func TestLLMSettingsRejectedInFileMode(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactoriesWithStoreMode(server.URL, "file"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_settings" "test" {
  gateways = ["default"]
}`,
				ExpectError: regexp.MustCompile(`requires config\.storage\.mode = hybrid`),
			},
		},
	})
}
