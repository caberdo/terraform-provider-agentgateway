package trafficgateway_test

import (
	"fmt"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	accGatewayName = "tf-acc-gateway"
	accGatewayType = "agentgateway_traffic_gateway"
)

// TestAccTrafficGateway_basic creates a gateway, imports it, updates its port
// and destroys it.
func TestAccTrafficGateway_basic(t *testing.T) {
	acctest.PreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"agentgateway": providerserver.NewProtocol6WithError(provider.New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficGatewayConfig(18080),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accGatewayType+".test", "id", accGatewayName),
					resource.TestCheckResourceAttr(accGatewayType+".test", "name", accGatewayName),
					resource.TestCheckResourceAttr(accGatewayType+".test", "port", "18080"),
					resource.TestCheckResourceAttr(accGatewayType+".test", "bind_address", "127.0.0.1"),
					resource.TestCheckResourceAttr(accGatewayType+".test", "protocol", "HTTP"),
				),
			},
			{
				ResourceName:      accGatewayType + ".test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testAccTrafficGatewayConfig(18081),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accGatewayType+".test", "port", "18081"),
				),
			},
		},
	})
}

// TestAccTrafficGateway_listeners creates a gateway with a named listener and
// verifies the nested block reads back.
func TestAccTrafficGateway_listeners(t *testing.T) {
	acctest.PreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"agentgateway": providerserver.NewProtocol6WithError(provider.New("test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficGatewayListenersConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accGatewayType+".listeners", "name", "tf-acc-listeners"),
					resource.TestCheckResourceAttr(accGatewayType+".listeners", "listeners.#", "1"),
					resource.TestCheckResourceAttr(accGatewayType+".listeners", "listeners.0.name", "public"),
					resource.TestCheckResourceAttr(accGatewayType+".listeners", "listeners.0.hostname", "example.com"),
					resource.TestCheckResourceAttr(accGatewayType+".listeners", "listeners.0.protocol", "HTTP"),
				),
			},
			{
				ResourceName:      accGatewayType + ".listeners",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccTrafficGatewayConfig(port int) string {
	return fmt.Sprintf(`
resource "agentgateway_traffic_gateway" "test" {
  name         = %q
  port         = %d
  bind_address = "127.0.0.1"
  protocol     = "HTTP"
}
`, accGatewayName, port)
}

const testAccTrafficGatewayListenersConfig = `
resource "agentgateway_traffic_gateway" "listeners" {
  name         = "tf-acc-listeners"
  port         = 18082
  bind_address = "127.0.0.1"

  listeners {
    name     = "public"
    hostname = "example.com"
    protocol = "HTTP"
  }
}
`
