package trafficroutes

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
	accRouteA = "acctest-traffic-route-a"
	accRouteB = "acctest-traffic-route-b"
)

// TestAccTrafficRoutesDataSource creates two routes in the same test and reads
// them back through the data source. It is compiled by `go test` and only runs
// when TF_ACC is set and the acceptance environment is reachable.
func TestAccTrafficRoutesDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkRoutesDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "a" {
  name = "` + accRouteA + `"

  value_json = jsonencode({
    name      = "` + accRouteA + `"
    gateways  = ["default"]
    hostnames = ["a.acctest.example"]
    backends = [
      { host = { host = "127.0.0.1", port = 8080 } },
    ]
  })
}

resource "agentgateway_test_resource" "b" {
  name = "` + accRouteB + `"

  value_json = jsonencode({
    name      = "` + accRouteB + `"
    gateways  = ["default"]
    hostnames = ["b.acctest.example"]
    backends = [
      { host = { host = "127.0.0.1", port = 8081 } },
    ]
  })
}

data "agentgateway_traffic_routes" "test" {
  name_prefix = "acctest-traffic-route"

  depends_on = [
    agentgateway_test_resource.a,
    agentgateway_test_resource.b,
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "id", "traffic.route:acctest-traffic-route"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.#", "2"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.name", accRouteA),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.0", "default"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.0", "a.acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.backends.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.name", accRouteB),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.hostnames.0", "b.acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.backends.#", "1"),
				),
			},
		},
	})
}

// checkRoutesDestroyed proves the backing routes are gone after destroy.
func checkRoutesDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindTrafficRoute)
		if err != nil {
			return fmt.Errorf("listing traffic routes: %w", err)
		}
		for _, res := range list {
			if res.ID == accRouteA || res.ID == accRouteB {
				return fmt.Errorf("traffic route %q still exists after destroy", res.ID)
			}
		}
		return nil
	}
}
