package runtime

import (
	"context"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccRuntimeDataSource reads the hybrid gateway's runtime document. It is
// compiled by `go test` and only runs when TF_ACC is set and the acceptance
// environment is reachable.
func TestAccRuntimeDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		Steps: []resource.TestStep{
			{
				Config: runtimeDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_runtime.test", "id", "runtime"),
					resource.TestCheckResourceAttrSet("data.agentgateway_runtime.test", "build_version"),
					resource.TestCheckResourceAttrSet("data.agentgateway_runtime.test", "git_revision"),
					resource.TestCheckResourceAttr("data.agentgateway_runtime.test", "gateway_mode", "standalone"),
					resource.TestCheckResourceAttr("data.agentgateway_runtime.test", "config_store_mode", "hybrid"),
					resource.TestCheckResourceAttr("data.agentgateway_runtime.test", "config_reload_synchronized", "true"),
				),
			},
		},
	})
}

const runtimeDataSourceConfig = `
data "agentgateway_runtime" "test" {}
`

// testFactories builds a provider that registers only this data source.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

// testProvider is the smallest provider that can host this data source in an
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
		MarkdownDescription: "Minimal provider used by this data source's acceptance test.",
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
	return []func() datasource.DataSource{New}
}

func (p *testProvider) Resources(_ context.Context) []func() frameworkresource.Resource {
	return nil
}
