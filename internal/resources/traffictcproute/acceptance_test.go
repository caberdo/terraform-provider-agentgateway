package traffictcproute_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/traffictcproute"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	accTCPRouteName   = "tf-acc-tcp-route"
	accTCPRouteType   = "agentgateway_traffic_tcp_route"
	accTCPGatewayName = "acctest-tcp-gateway"
)

// TestAccTrafficTCPRoute_basic creates a TCP route, imports it, updates its
// backends, and destroys it.
func TestAccTrafficTCPRoute_basic(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	// A TCP route may only attach to a gateway that has TCP listeners, and the
	// acceptance fixture's `default` gateway is HTTP. Seed a TCP gateway for
	// the duration of the test.
	seedTCPGateway(t, endpoint)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkTCPRouteDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficTCPRouteConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "id", accTCPRouteName),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "name", accTCPRouteName),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "namespace", "default"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "rule_name", "tcp"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "gateways.#", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "hostnames.#", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.#", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.weight", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.host", "127.0.0.1:5432"),
				),
			},
			{
				ResourceName:      accTCPRouteType + ".test",
				ImportState:       true,
				ImportStateId:     accTCPRouteName,
				ImportStateVerify: true,
				// Read preserves JSON leaves the configuration left null, and a
				// gateway may normalize or omit the route-level policies.
				ExpectNonEmptyPlan:      true,
				ImportStateVerifyIgnore: []string{"policies_json"},
			},
			{
				Config: testAccTrafficTCPRouteUpdatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.#", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.weight", "2"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.service_json", `{"name":"default/postgres","port":5432}`),
				),
			},
		},
	})
}

// TestTrafficTCPRouteCRUD drives the resource through Terraform against the
// protocol fake: create with a defaulted weight, import, update and destroy.
func TestTrafficTCPRouteCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkTCPRouteDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficTCPRouteConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "id", accTCPRouteName),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "gateways.#", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "hostnames.#", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.weight", "1"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.host", "127.0.0.1:5432"),
				),
			},
			{
				ResourceName:      accTCPRouteType + ".test",
				ImportState:       true,
				ImportStateId:     accTCPRouteName,
				ImportStateVerify: true,
			},
			{
				Config: testAccTrafficTCPRouteUpdatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.weight", "2"),
					resource.TestCheckResourceAttr(accTCPRouteType+".test", "backends.0.service_json", `{"name":"default/postgres","port":5432}`),
				),
			},
		},
	})
}

const testAccTrafficTCPRouteConfig = `
resource "agentgateway_traffic_tcp_route" "test" {
  name      = "` + accTCPRouteName + `"
  gateways  = ["` + accTCPGatewayName + `"]
  namespace = "default"
  rule_name = "tcp"
  hostnames = ["tcp.example.com"]

  # The gateway's TCP backend host is a single "host:port" string.
  backends {
    host = "127.0.0.1:5432"
  }
}
`

const testAccTrafficTCPRouteUpdatedConfig = `
resource "agentgateway_traffic_tcp_route" "test" {
  name      = "` + accTCPRouteName + `"
  gateways  = ["` + accTCPGatewayName + `"]
  namespace = "default"
  rule_name = "tcp"
  hostnames = ["tcp.example.com"]

  backends {
    weight = 2

    service_json = jsonencode({
      name = "default/postgres"
      port = 5432
    })
  }
}
`

// seedTCPGateway creates the TCP gateway the route attaches to and removes it
// after the test. The collection PUT is an upsert, so a gateway left behind by
// an aborted run is reused instead of failing the seed.
func seedTCPGateway(t *testing.T, endpoint string) {
	t.Helper()

	c, err := client.New(client.Config{
		BaseURL:    endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/acctest",
	})
	if err != nil {
		t.Fatalf("creating acceptance client: %v", err)
	}

	value := json.RawMessage(`{"name":"` + accTCPGatewayName + `","port":14099,"protocol":"TCP"}`)
	if _, err := c.Create(context.Background(), configresource.KindTrafficGateway, value); err != nil {
		t.Fatalf("seeding TCP gateway %s: %v", accTCPGatewayName, err)
	}
	t.Cleanup(func() {
		if err := c.Delete(context.Background(), configresource.KindTrafficGateway, accTCPGatewayName); err != nil && !errors.Is(err, client.ErrNotFound) {
			t.Logf("removing seeded TCP gateway %s: %v", accTCPGatewayName, err)
		}
	})
}

// testFactories builds a provider that registers only this resource. The
// resource package must not import internal/provider (design §4), so the
// acceptance test supplies the minimal provider it needs.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

// checkTCPRouteDestroyed fails when the route outlives the test.
func checkTCPRouteDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindTrafficTCPRoute)
		if err != nil {
			return fmt.Errorf("listing traffic TCP routes: %w", err)
		}
		for _, res := range list {
			if res.ID == accTCPRouteName {
				return fmt.Errorf("traffic TCP route %q still exists after destroy", res.ID)
			}
		}
		return nil
	}
}

// testProvider is the smallest provider that can host this resource in an
// acceptance test.
type testProvider struct {
	endpoint string
}

func (p *testProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "agentgateway"
	resp.Version = "acctest"
}

func (p *testProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		MarkdownDescription: "Minimal provider used by this resource's acceptance test.",
	}
}

func (p *testProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	c, err := client.New(client.Config{
		BaseURL:    p.endpoint,
		HTTPClient: http.DefaultClient,
		UserAgent:  "terraform-provider-agentgateway/acctest",
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create the acceptance client", err.Error())
		return
	}

	data := &providerdata.Data{Client: c}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *testProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{traffictcproute.New}
}
