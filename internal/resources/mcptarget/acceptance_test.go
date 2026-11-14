package mcptarget

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

const acceptanceName = "acctest-mcp-target"

// TestAccMCPTargetResource covers create, import, update and destroy against
// the hybrid gateway. It is compiled by `go test` and only runs when TF_ACC is
// set and the acceptance environment is reachable.
func TestAccMCPTargetResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkTargetDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: httpTargetConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "id", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "name", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.host", "mcp.example.com"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.port", "8080"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.path", "/mcp"),
					resource.TestCheckResourceAttrSet("agentgateway_mcp_target.test", "policies_json"),
				),
			},
			{
				ResourceName:      "agentgateway_mcp_target.test",
				ImportState:       true,
				ImportStateId:     acceptanceName,
				ImportStateVerify: true,
				// The gateway returns the stored target unchanged (management
				// API contract), so every attribute is verifiable. The plan
				// after import is allowed to be non-empty because a gateway
				// release may normalize defaulted transport parameters.
				ExpectNonEmptyPlan: true,
			},
			{
				// Rename is an in-place update (spec C7): the value goes to the
				// old id and the server returns the new one, which the harness
				// stores as the computed id.
				Config: renamedTargetConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "id", acceptanceName+"-renamed"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "name", acceptanceName+"-renamed"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "mcp.path", "/mcp/v2"),
				),
			},
			{
				// A different transport replaces the block the server returned,
				// so no stale host survives.
				Config: stdioTargetConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "stdio.cmd", "npx"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "stdio.args.#", "2"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_target.test", "mcp.host"),
				),
			},
		},
	})
}

// TestAccMCPTargetOpenAPITransport covers the openapi transport with its
// inline schema, which is the only transport that carries a JSON leaf.
func TestAccMCPTargetOpenAPITransport(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkTargetDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_target" "test" {
  name = "acctest-mcp-target-openapi"

  openapi = {
    host = "api.example.com"
    port = 443
    path = "/openapi.json"

    schema_json = jsonencode({
      url = "https://api.example.com/openapi.json"
    })
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "id", "acctest-mcp-target-openapi"),
					resource.TestCheckResourceAttr("agentgateway_mcp_target.test", "openapi.host", "api.example.com"),
					resource.TestCheckResourceAttrSet("agentgateway_mcp_target.test", "openapi.schema_json"),
				),
			},
		},
	})
}

const httpTargetConfig = `
resource "agentgateway_mcp_target" "test" {
  name      = "acctest-mcp-target"
  condition = "true"

  mcp = {
    host = "mcp.example.com"
    port = 8080
    path = "/mcp"
  }

  policies_json = jsonencode({
    cors = { allowOrigins = ["*"] }
  })
}
`

const renamedTargetConfig = `
resource "agentgateway_mcp_target" "test" {
  name      = "acctest-mcp-target-renamed"
  condition = "true"

  mcp = {
    host = "mcp.example.com"
    port = 8080
    path = "/mcp/v2"
  }

  policies_json = jsonencode({
    cors = { allowOrigins = ["*"] }
  })
}
`

const stdioTargetConfig = `
resource "agentgateway_mcp_target" "test" {
  name = "acctest-mcp-target-renamed"

  stdio = {
    cmd       = "npx"
    args      = ["-y", "@modelcontextprotocol/server-filesystem"]
    env       = { TOKEN = "$ACCTEST_TOKEN" }
    clear_env = true
  }
}
`

// testFactories builds a provider that registers only this resource. The
// resource package must not import internal/provider (design §4), so the
// acceptance test supplies the minimal provider it needs.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint, storeMode: "hybrid"}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

func checkTargetDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindMCPTarget)
		if err != nil {
			return fmt.Errorf("listing mcp targets: %w", err)
		}
		for _, res := range list {
			if res.ID == acceptanceName || res.ID == acceptanceName+"-renamed" {
				return fmt.Errorf("mcp target %q still exists after destroy", res.ID)
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
