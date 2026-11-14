package trafficroute_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/trafficroute"
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
	accRouteName = "tf-acc-route"
	accRouteType = "agentgateway_traffic_route"
)

// TestAccTrafficRoute_basic creates a route, imports it, updates its matches
// and backends, and destroys it.
func TestAccTrafficRoute_basic(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkRouteDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficRouteConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accRouteType+".test", "id", accRouteName),
					resource.TestCheckResourceAttr(accRouteType+".test", "name", accRouteName),
					resource.TestCheckResourceAttr(accRouteType+".test", "namespace", "default"),
					resource.TestCheckResourceAttr(accRouteType+".test", "rule_name", "api"),
					resource.TestCheckResourceAttr(accRouteType+".test", "gateways.#", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "hostnames.#", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.#", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.path.0.prefix", "/api"),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.method", "GET"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.#", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.weight", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.host", "127.0.0.1:8080"),
				),
			},
			{
				ResourceName:      accRouteType + ".test",
				ImportState:       true,
				ImportStateId:     accRouteName,
				ImportStateVerify: true,
				// Read preserves JSON leaves the configuration left null, and a
				// gateway may normalize or omit route-level policies, the
				// namespace/rule name, hostnames and the defaulted weight.
				ExpectNonEmptyPlan:      true,
				ImportStateVerifyIgnore: []string{"policies_json", "rule_name", "namespace", "hostnames", "weight", "backends.0.weight"},
			},
			{
				Config: testAccTrafficRouteUpdatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.path.0.exact", "/v1/health"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.#", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.weight", "2"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.service_json", `{"name":"default/svc","port":8080}`),
				),
			},
		},
	})
}

// TestTrafficRouteCRUD drives the resource through Terraform against the
// protocol fake: create with a defaulted weight, import, update and destroy.
func TestTrafficRouteCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkRouteDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: testAccTrafficRouteConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accRouteType+".test", "id", accRouteName),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.#", "1"),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.headers.0.name", "x-tenant"),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.headers.0.exact", "acme"),
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.query.0.regex", "^[0-9]+$"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.weight", "1"),
				),
			},
			{
				ResourceName:      accRouteType + ".test",
				ImportState:       true,
				ImportStateId:     accRouteName,
				ImportStateVerify: true,
			},
			{
				Config: testAccTrafficRouteUpdatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accRouteType+".test", "matches.0.path.0.exact", "/v1/health"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.weight", "2"),
					resource.TestCheckResourceAttr(accRouteType+".test", "backends.0.service_json", `{"name":"default/svc","port":8080}`),
				),
			},
		},
	})
}

const testAccTrafficRouteConfig = `
resource "agentgateway_traffic_route" "test" {
  name      = "` + accRouteName + `"
  gateways  = ["default"]
  namespace = "default"
  rule_name = "api"
  hostnames = ["example.com"]

  matches {
    path {
      prefix = "/api"
    }
    method = "GET"

    headers {
      name  = "x-tenant"
      exact = "acme"
    }

    query {
      name  = "page"
      regex = "^[0-9]+$"
    }
  }

  backends {
    host = "127.0.0.1:8080"
  }
}
`

const testAccTrafficRouteUpdatedConfig = `
resource "agentgateway_traffic_route" "test" {
  name      = "` + accRouteName + `"
  gateways  = ["default"]
  namespace = "default"
  rule_name = "api"
  hostnames = ["example.com"]

  matches {
    path {
      exact = "/v1/health"
    }
  }

  backends {
    weight = 2

    # A service reference is a NamespacedHostname (namespace/hostname).
    service_json = jsonencode({
      name = "default/svc"
      port = 8080
    })
  }
}
`

// testFactories builds a provider that registers only this resource. The
// resource package must not import internal/provider (design §4), so the
// acceptance test supplies the minimal provider it needs.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

// checkRouteDestroyed fails when the route outlives the test.
func checkRouteDestroyed(endpoint string) resource.TestCheckFunc {
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
			if res.ID == accRouteName {
				return fmt.Errorf("traffic route %q still exists after destroy", res.ID)
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
	return []func() frameworkresource.Resource{trafficroute.New}
}
