package trafficroutes

import (
	"encoding/json"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestTrafficRoutesDataSource pins the listing contract: deterministic order
// by id, the name_prefix filter and the computed "<kind>:<prefix>" id.
func TestTrafficRoutesDataSource(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "a" {
  name = "route-a"

  value_json = jsonencode({
    name      = "route-a"
    gateways  = ["default"]
    hostnames = ["a.example"]
    backends = [
      { host = { host = "127.0.0.1", port = 8080 } },
    ]
  })
}

resource "agentgateway_test_resource" "b" {
  name = "route-b"

  value_json = jsonencode({
    name      = "route-b"
    gateways  = ["default", "other"]
    hostnames = ["b.example"]
    backends = [
      { weight = 2, service = { name = "svc", namespace = "default" } },
      { host = { host = "10.0.0.1", port = 9090 } },
    ]
  })
}

resource "agentgateway_test_resource" "other" {
  name       = "other-route"
  value_json = jsonencode({ name = "other-route" })
}

data "agentgateway_traffic_routes" "test" {
  name_prefix = "route-"

  depends_on = [
    agentgateway_test_resource.a,
    agentgateway_test_resource.b,
    agentgateway_test_resource.other,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "id", "traffic.route:route-"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.#", "2"),
					// Sorted by id: route-a before route-b.
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.name", "route-a"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.0", "default"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.0", "a.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.backends.#", "1"),
					resource.TestCheckResourceAttrWith("data.agentgateway_traffic_routes.test", "routes.0.backends.0", func(value string) error {
						var backend map[string]any
						return json.Unmarshal([]byte(value), &backend)
					}),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.name", "route-b"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.gateways.#", "2"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.gateways.1", "other"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.hostnames.0", "b.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.backends.#", "2"),
				),
			},
		},
	})
}
