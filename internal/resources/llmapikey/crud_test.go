package llmapikey

import (
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLLMAPIKeyCRUD drives the resource through Terraform against the protocol
// fake: create, import and update. It pins the server-generated id, the
// null-versus-empty allowed_models handling and the budgets shape end to end.
func TestLLMAPIKeyCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkAPIKeyDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_api_key" "test" {
  key      = "sk-acctest-crud-0001"
  metadata = { team = "` + acceptanceTeam + `" }

  allowed_models = ["gpt-4o-mini"]

  budgets = [
    {
      name               = "daily-spend"
      limit              = { unit = "USD", amount = 50 }
      window             = { rolling = "24h" }
      on_budget_exceeded = "Block"
    },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("agentgateway_llm_api_key.test", "id"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "revision", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "key", "sk-acctest-crud-0001"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "metadata.team", acceptanceTeam),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "allowed_models.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "allowed_models.0", "gpt-4o-mini"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.0.limit.amount", "50"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_api_key.test",
				ImportState:       true,
				ImportStateIdFunc: apiKeyImportID,
				ImportStateVerify: true,
			},
			{
				Config: `
resource "agentgateway_llm_api_key" "test" {
  key      = "sk-acctest-crud-0001-rotated"
  metadata = { team = "` + acceptanceTeam + `" }

  allowed_models = []
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("agentgateway_llm_api_key.test", "id"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "revision", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "key", "sk-acctest-crud-0001-rotated"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "allowed_models.#", "0"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_api_key.test", "budgets"),
				),
			},
		},
	})
}
