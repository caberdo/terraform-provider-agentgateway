package trafficgateways

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccTrafficGatewaysDataSource creates two gateways in the same test and
// reads them back through the data source. It is compiled by `go test` and only
// runs when TF_ACC is set and the acceptance environment is reachable.
//
// Names carry a per-run suffix: the acceptance gateway is shared, so a stale
// gateway left by a failed run must not change this test's counts.
func TestAccTrafficGatewaysDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	suffix := time.Now().UnixNano()
	prefix := fmt.Sprintf("acctest-traffic-gateway-%d", suffix)
	listenerName := prefix + "-listener"
	plainName := prefix + "-plain"
	// A per-run port keeps a stale gateway from the same run family off the
	// port this test wants.
	port := 20000 + int(suffix%10000)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkGatewaysDestroyed(endpoint, listenerName, plainName),
		Steps: []resource.TestStep{
			{
				Config: gatewayConfig(prefix, listenerName, plainName, port),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "id", "traffic.gateway:"+prefix),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.#", "2"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.name", listenerName),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.name", "http"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.hostname", "acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.0.listeners.0.protocol", "HTTP"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.name", plainName),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.port", fmt.Sprintf("%d", port)),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.bind_address", "0.0.0.0"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_gateways.test", "gateways.1.protocol", "HTTP"),
				),
			},
		},
	})
}

// gatewayConfig builds the acceptance configuration for the two gateways and
// the listing.
func gatewayConfig(prefix, listenerName, plainName string, port int) string {
	return fmt.Sprintf(`
resource "agentgateway_test_resource" "listener" {
  name = %q

  value_json = jsonencode({
    name = %q
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
  name = %q

  value_json = jsonencode({
    name        = %q
    port        = %d
    bindAddress = "0.0.0.0"
    protocol    = "HTTP"
  })
}

data "agentgateway_traffic_gateways" "test" {
  name_prefix = %q

  depends_on = [
    agentgateway_test_resource.listener,
    agentgateway_test_resource.plain,
  ]
}
`, listenerName, listenerName, plainName, plainName, port, prefix)
}

// checkGatewaysDestroyed proves the backing gateways are gone after destroy.
func checkGatewaysDestroyed(endpoint string, names ...string) resource.TestCheckFunc {
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
			for _, name := range names {
				if res.ID == name {
					return fmt.Errorf("traffic gateway %q still exists after destroy", res.ID)
				}
			}
		}
		return nil
	}
}
