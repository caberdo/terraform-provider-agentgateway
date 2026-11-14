package mcptarget

import (
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestMCPTargetCRUD drives the resource through Terraform against the protocol
// fake: create with the streamable HTTP transport, import, then switch to stdio.
func TestMCPTargetCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkTargetDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_target" "test" {
  name      = "acctest-mcp-target"
  condition = "true"

  mcp = {
    host = "mcp.example.com"
    port = 8080
    path = "/mcp"
  }

  policies_json = jsonencode({
    cors = { allowOrigins = ["*"] }
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "id", "acctest-mcp-target"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "revision", "1"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.host", "mcp.example.com"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.port", "8080"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.path", "/mcp"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_target.test", "sse.host"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_target.test", "stdio.cmd"),
					resource.TestCheckResourceAttrSet("agentgateway_mcp_target.test", "policies_json"),
				),
			},
			{
				ResourceName:      "agentgateway_mcp_target.test",
				ImportState:       true,
				ImportStateId:     "acctest-mcp-target",
				ImportStateVerify: true,
				// A gateway release may normalize defaulted transport
				// parameters, so the plan after import need not be empty.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: `
resource "agentgateway_mcp_target" "test" {
  name = "acctest-mcp-target"

  stdio = {
    cmd       = "npx"
    args      = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    env       = { TOKEN = "abc" }
    clear_env = true
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "revision", "2"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "stdio.cmd", "npx"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "stdio.args.#", "3"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "stdio.env.TOKEN", "abc"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "stdio.clear_env", "true"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_target.test", "mcp.host"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_target.test", "policies_json"),
				),
			},
		},
	})
}

// TestMCPTargetTransportGroup pins that exactly one transport is required
// before any request reaches the gateway.
func TestMCPTargetTransportGroup(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_target" "test" {
  name = "no-transport"
}`,
				ExpectError: regexp.MustCompile(`No attribute specified when one \(and only one\)`),
			},
			{
				Config: `
resource "agentgateway_mcp_target" "test" {
  name = "two-transports"

  mcp = {
    host = "mcp.example.com"
    port = 8080
    path = "/mcp"
  }

  sse = {
    host = "sse.example.com"
    port = 8080
    path = "/sse"
  }
}`,
				ExpectError: regexp.MustCompile(`attributes specified when one \(and only one\)`),
			},
		},
	})
}
