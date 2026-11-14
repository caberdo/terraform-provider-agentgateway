package trafficroutes

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

// TestAccTrafficRoutesDataSource creates two routes in the same test and reads
// them back through the data source. It is compiled by `go test` and only runs
// when TF_ACC is set and the acceptance environment is reachable.
//
// Names carry a per-run suffix: the acceptance gateway is shared, so a stale
// route left by a failed run must not change this test's counts.
func TestAccTrafficRoutesDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	suffix := time.Now().UnixNano()
	prefix := fmt.Sprintf("acctest-traffic-route-%d", suffix)
	routeA := prefix + "-a"
	routeB := prefix + "-b"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkRoutesDestroyed(endpoint, routeA, routeB),
		Steps: []resource.TestStep{
			{
				Config: routesConfig(prefix, routeA, routeB),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "id", "traffic.route:"+prefix),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.#", "2"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.name", routeA),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.gateways.0", "default"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.hostnames.0", "a.acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.0.backends.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.name", routeB),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.hostnames.0", "b.acctest.example"),
					resource.TestCheckResourceAttr("data.agentgateway_traffic_routes.test", "routes.1.backends.#", "1"),
				),
			},
		},
	})
}

// routesConfig builds the acceptance configuration for the two routes and the
// listing.
func routesConfig(prefix, routeA, routeB string) string {
	return fmt.Sprintf(`
resource "agentgateway_test_resource" "a" {
  name = %q

  value_json = jsonencode({
    name      = %q
    gateways  = ["default"]
    hostnames = ["a.acctest.example"]
    backends = [
      { host = "127.0.0.1:8080" },
    ]
  })
}

resource "agentgateway_test_resource" "b" {
  name = %q

  value_json = jsonencode({
    name      = %q
    gateways  = ["default"]
    hostnames = ["b.acctest.example"]
    backends = [
      { host = "127.0.0.1:8081" },
    ]
  })
}

data "agentgateway_traffic_routes" "test" {
  name_prefix = %q

  depends_on = [
    agentgateway_test_resource.a,
    agentgateway_test_resource.b,
  ]
}
`, routeA, routeA, routeB, routeB, prefix)
}

// checkRoutesDestroyed proves the backing routes are gone after destroy.
func checkRoutesDestroyed(endpoint string, names ...string) resource.TestCheckFunc {
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
			for _, name := range names {
				if res.ID == name {
					return fmt.Errorf("traffic route %q still exists after destroy", res.ID)
				}
			}
		}
		return nil
	}
}
