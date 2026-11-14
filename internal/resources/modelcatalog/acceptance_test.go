package modelcatalog

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

// TestAccModelCatalogResource covers create, import, update and destroy of the
// singleton against the hybrid gateway. It is compiled by `go test` and only
// runs when TF_ACC is set and the acceptance environment is reachable.
func TestAccModelCatalogResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkCatalogDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: customCatalogConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_model_catalog.test", "id", "default"),
					resource.TestCheckResourceAttrSet("agentgateway_model_catalog.test", "revision"),
					resource.TestCheckResourceAttrSet("agentgateway_model_catalog.test", "custom_json"),
					resource.TestCheckNoResourceAttr("agentgateway_model_catalog.test", "base_json"),
				),
			},
			{
				// The singleton only exists after its first apply, so the
				// import follows the create. Both leaves are echoed back, but
				// the plan after an import of a singleton is not guaranteed to
				// be empty, so the step allows it.
				ResourceName:       "agentgateway_model_catalog.test",
				ImportState:        true,
				ImportStateId:      "default",
				ImportStateVerify:  true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: baseCatalogConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_model_catalog.test", "id", "default"),
					resource.TestCheckResourceAttrSet("agentgateway_model_catalog.test", "base_json"),
					resource.TestCheckNoResourceAttr("agentgateway_model_catalog.test", "custom_json"),
				),
			},
		},
	})
}

const customCatalogConfig = `
resource "agentgateway_model_catalog" "test" {
  custom_json = jsonencode({
    providers = {
      internal = {
        models = {
          "acme-large" = {
            rates = { input = "1.00", output = "2.00" }
          }
        }
      }
    }
  })
}
`

const baseCatalogConfig = `
resource "agentgateway_model_catalog" "test" {
  base_json = jsonencode({
    metadata  = { generatedAt = "2026-11-14T00:00:00Z" }
    providers = {}
  })
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

func checkCatalogDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindModelCatalog)
		if err != nil {
			return fmt.Errorf("listing model catalogs: %w", err)
		}
		for _, res := range list {
			if res.ID == "default" {
				return fmt.Errorf("model catalog %q still exists after destroy", res.ID)
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

	// The catalog is a singleton: the harness rejects it in file mode because
	// the list omits it there (spec C24). The acceptance gateway is hybrid.
	data := &providerdata.Data{Client: c, StoreMode: "hybrid"}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *testProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{New}
}
