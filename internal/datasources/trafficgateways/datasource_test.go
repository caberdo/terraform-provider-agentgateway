package trafficgateways

import (
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestTrafficGatewaysDataSource pins the listing contract: deterministic order
// by id, the name_prefix filter and the computed "<kind>:<prefix>" id.
func TestTrafficGatewaysDataSource(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "listener" {
  name = "zz-listener"

  value_json = jsonencode({
    name = "zz-listener"
    listeners = [
      {
        name     = "https"
        hostname = "acctest.example"
        protocol = "HTTPS"
      },
    ]
  })
}

resource "agentgateway_test_resource" "plain" {
  name = "zz-plain"

  value_json = jsonencode({
    name        = "zz-plain"
    port        = 8080
    bindAddress = "0.0.0.0"
    protocol    = "HTTP"
  })
}

resource "agentgateway_test_resource" "other" {
  name       = "other-gateway"
  value_json = jsonencode({ name = "other-gateway" })
}

data "agentgateway_traffic_gateways" "test" {
  name_prefix = "zz-"

  depends_on = [
    agentgateway_test_resource.listener,
    agentgateway_test_resource.plain,
    agentgateway_test_resource.other,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "id", "traffic.gateway:zz-"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.#", "2"),
					// Sorted by id: zz-listener before zz-plain.
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.name", "zz-listener"),
					resource.TestCheckNoResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.port"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.name", "https"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.hostname", "acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.protocol", "HTTPS"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.name", "zz-plain"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.port", "8080"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.bind_address", "0.0.0.0"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.protocol", "HTTP"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.listeners.#", "0"),
				),
			},
			{
				// Without a prefix the listing keeps every gateway, still sorted.
				Config: `
resource "agentgateway_test_resource" "listener" {
  name = "zz-listener"

  value_json = jsonencode({
    name = "zz-listener"
    listeners = [
      {
        name     = "https"
        hostname = "acctest.example"
        protocol = "HTTPS"
      },
    ]
  })
}

resource "agentgateway_test_resource" "plain" {
  name = "zz-plain"

  value_json = jsonencode({
    name        = "zz-plain"
    port        = 8080
    bindAddress = "0.0.0.0"
    protocol    = "HTTP"
  })
}

resource "agentgateway_test_resource" "other" {
  name       = "other-gateway"
  value_json = jsonencode({ name = "other-gateway" })
}

data "agentgateway_traffic_gateways" "test" {
  depends_on = [
    agentgateway_test_resource.listener,
    agentgateway_test_resource.plain,
    agentgateway_test_resource.other,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "id", "traffic.gateway:"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.#", "3"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.name", "other-gateway"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.2.name", "zz-plain"),
				),
			},
		},
	})
}
