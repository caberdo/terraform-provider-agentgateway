package llmsettings

import (
	"context"
	"fmt"
	"net/http"
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

// TestAccLLMSettingsResource covers create, import, update and destroy against
// the hybrid gateway. It is compiled by `go test` and only runs when TF_ACC is
// set and the acceptance environment is reachable.
func TestAccLLMSettingsResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkSettingsDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: gatewaysConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "id", "default"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.0", "default"),
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "gateways.1", "llm/listener"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_settings.test",
				ImportState:       true,
				ImportStateId:     "default",
				ImportStateVerify: true,
			},
			{
				Config: portConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_settings.test", "port", "8080"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_settings.test", "gateways.#"),
				),
			},
		},
	})
}

const gatewaysConfig = `
resource "agentgateway_llm_settings" "test" {
  gateways = ["default", "llm/listener"]
}
`

const portConfig = `
resource "agentgateway_llm_settings" "test" {
  port = 8080

  tls_json = jsonencode({
    cert = "/etc/tls/tls.crt"
    key  = "/etc/tls/tls.key"
  })
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

		list, err := c.List(context.Background(), configresource.KindLLMSettings)
		if err != nil {
			return fmt.Errorf("listing llm settings: %w", err)
		}
		for _, res := range list {
			if res.ID == "default" {
				return fmt.Errorf("llm settings %q still exists after destroy", res.ID)
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
