package mcpsettings

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccMCPSettingsResource covers create, import, update and destroy against
// the hybrid gateway. It is compiled by `go test` and only runs when TF_ACC is
// set and the acceptance environment is reachable.
func TestAccMCPSettingsResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkSettingsDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: gatewaysSettingsConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "id", "default"),
					// The gateway resolves every reference against the merged
					// config, so only the fixture's real `default` gateway can
					// be attached here.
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "gateways.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "gateways.0", "default"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "stateful_mode", "stateless"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "prefix_mode", "always"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "server.name", "agentgateway"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "server.version", "1.7.0"),
				),
			},
			{
				ResourceName:      "agentgateway_mcp_settings.test",
				ImportState:       true,
				ImportStateId:     "default",
				ImportStateVerify: true,
				// A gateway release may materialize defaults for fields the
				// configuration omits (gateways, stateful_mode,
				// dns_rebinding_protection), so the plan after import need not
				// be empty. The attributes that are set are still verified.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: portSettingsConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "port", "8080"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "prefix_mode", "conditional"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "failure_mode", "failOpen"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "sse_keep_alive", "15s"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "dns_rebinding_protection", "false"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_settings.test", "gateways.#"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_settings.test", "server.name"),
				),
			},
		},
	})
}

// TestAccMCPSettingsFileOwnedField covers the 409 the gateway returns when the
// config file owns a field (spec C9): the diagnostic must say so, not report a
// generic conflict. It runs against the file-owned gateway.
func TestAccMCPSettingsFileOwnedField(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleFileOwned)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_settings" "test" {
  stateful_mode = "stateful"
}`,
				ExpectError: regexp.MustCompile(`config file owns the statefulMode field`),
			},
		},
	})
}

const gatewaysSettingsConfig = `
resource "agentgateway_mcp_settings" "test" {
  gateways                 = ["default"]
  stateful_mode            = "stateless"
  prefix_mode              = "always"
  failure_mode             = "failClosed"
  sse_keep_alive           = "30s"
  dns_rebinding_protection = true

  server = {
    name         = "agentgateway"
    version      = "1.7.0"
    title        = "Gateway"
    instructions = "Use tools."
  }
}
`

const portSettingsConfig = `
resource "agentgateway_mcp_settings" "test" {
  port                     = 8080
  prefix_mode              = "conditional"
  failure_mode             = "failOpen"
  sse_keep_alive           = "15s"
  dns_rebinding_protection = false
}
`

// testFactories builds a provider that registers only this resource. The
// resource package must not import internal/provider (design §4), so the
// acceptance test supplies the minimal provider it needs. Settings are a
// hybrid-only singleton, so the provider reports the hybrid store mode.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	return testFactoriesWithStoreMode(endpoint, "hybrid")
}

// testFactoriesWithStoreMode is testFactories with an explicit store mode, so
// unit tests can exercise the file-mode rejection.
func testFactoriesWithStoreMode(endpoint, storeMode string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint, storeMode: storeMode}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

func checkSettingsDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindMCPSettings)
		if err != nil {
			return fmt.Errorf("listing mcp settings: %w", err)
		}
		for _, res := range list {
			if res.ID == "default" {
				return fmt.Errorf("mcp settings %q still exists after destroy", res.ID)
			}
		}
		return nil
	}
}

// testProvider is the smallest provider that can host this resource in an
// acceptance test.
type testProvider struct {
	endpoint  string
	storeMode string
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

	data := &providerdata.Data{Client: c, StoreMode: p.storeMode}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *testProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{New}
}
