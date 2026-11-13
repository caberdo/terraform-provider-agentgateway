package trafficgateway_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resources/trafficgateway"
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
	accGatewayName = "tf-acc-gateway"
	accGatewayType = "agentgateway_traffic_gateway"
)

// TestAccTrafficGateway_basic creates a gateway, imports it, updates its port
// and destroys it.
func TestAccTrafficGateway_basic(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkGatewayDestroyed(endpoint),
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
				ImportStateId:     accGatewayName,
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
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkGatewayDestroyed(endpoint),
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
				ImportStateId:     "tf-acc-listeners",
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

// testFactories builds a provider that registers only this resource. The
// resource package must not import internal/provider (design §4), so the
// acceptance test supplies the minimal provider it needs.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

// checkGatewayDestroyed fails when either gateway outlives the test.
func checkGatewayDestroyed(endpoint string) resource.TestCheckFunc {
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
			return fmt.Errorf("listing gateways: %w", err)
		}
		for _, res := range list {
			if res.ID == accGatewayName || res.ID == "tf-acc-listeners" {
				return fmt.Errorf("gateway %q still exists after destroy", res.ID)
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
	return []func() frameworkresource.Resource{trafficgateway.New}
}
