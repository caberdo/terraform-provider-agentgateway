package frontendpolicy

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

// TestAccFrontendPolicyResource covers create, import, update and destroy of a
// policy against the hybrid gateway. It compiles everywhere and only runs when
// TF_ACC is set and the acceptance environment is reachable.
func TestAccFrontendPolicyResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkPolicyDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_frontend_policy" "test" {
  name = "accessLog"
  spec = jsonencode({
    format = "json"
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_frontend_policy.test", "id", "accessLog"),
					resource.TestCheckResourceAttrSet("agentgateway_frontend_policy.test", "revision"),
				),
			},
			{
				// Policy bodies are stored verbatim, so the import must match the
				// managed state exactly; there is nothing to ignore.
				ResourceName:      "agentgateway_frontend_policy.test",
				ImportState:       true,
				ImportStateId:     "accessLog",
				ImportStateVerify: true,
			},
			{
				Config: `
resource "agentgateway_frontend_policy" "test" {
  name = "accessLog"
  spec = jsonencode({
    format = "json"
    filter = "status >= 400"
  })
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_frontend_policy.test", "id", "accessLog"),
				),
			},
		},
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

// checkPolicyDestroyed fails when a policy survives destroy.
func checkPolicyDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindFrontendPolicy)
		if err != nil {
			return fmt.Errorf("listing frontend policies: %w", err)
		}
		for _, res := range list {
			if res.ID == "accessLog" {
				return fmt.Errorf("frontend policy %q still exists after destroy", res.ID)
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
