package trafficgateways

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	accListenerName = "acctest-traffic-gateway-listener"
	accPlainName    = "acctest-traffic-gateway-plain"
)

// TestAccTrafficGatewaysDataSource creates two gateways in the same test and
// reads them back through the data source. It is compiled by `go test` and only
// runs when TF_ACC is set and the acceptance environment is reachable.
func TestAccTrafficGatewaysDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkGatewaysDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "listener" {
  name = "` + accListenerName + `"

  value_json = jsonencode({
    name = "` + accListenerName + `"
    listeners = [
      {
        name     = "http"
        hostname = "acctest.example"
        protocol = "HTTP"
      },
    ]
  })
}

resource "agentgateway_test_resource" "plain" {
  name = "` + accPlainName + `"

  value_json = jsonencode({
    name        = "` + accPlainName + `"
    port        = 18090
    bindAddress = "0.0.0.0"
    protocol    = "HTTP"
  })
}

data "agentgateway_traffic_gateways" "test" {
  name_prefix = "acctest-traffic-gateway"

  depends_on = [
    agentgateway_test_resource.listener,
    agentgateway_test_resource.plain,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "id", "traffic.gateway:acctest-traffic-gateway"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.#", "2"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.name", accListenerName),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.name", "http"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.hostname", "acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.protocol", "HTTP"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.name", accPlainName),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.port", "18090"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.bind_address", "0.0.0.0"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.protocol", "HTTP"),
				),
			},
		},
	})
}

// checkGatewaysDestroyed proves the backing gateways are gone after destroy.
func checkGatewaysDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindTrafficGateway)
		if err != nil {
			return fmt.Errorf("listing traffic gateways: %w", err)
		}
		for _, res := range list {
			if res.ID == accListenerName || res.ID == accPlainName {
				return fmt.Errorf("traffic gateway %q still exists after destroy", res.ID)
			}
		}
		return nil
	}
}
