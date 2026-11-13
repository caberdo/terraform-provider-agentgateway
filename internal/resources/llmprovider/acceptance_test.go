package llmprovider

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

const acceptanceName = "acctest-llm-provider"

// TestAccLLMProviderResource covers create, import, update and destroy against
// the hybrid gateway. It is compiled by `go test` and only runs when TF_ACC is
// set and the acceptance environment is reachable.
func TestAccLLMProviderResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkProviderDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: builtinConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "id", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "name", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "provider_name", "openAI"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "model", "gpt-4o"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "base_url", "https://api.openai.com"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_provider.test",
				ImportState:       true,
				ImportStateId:     acceptanceName,
				ImportStateVerify: true,
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "model", "gpt-4o-mini"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "path_prefix", "/openai"),
				),
			},
			{
				// Rename is an in-place update (spec C7): the value goes to the
				// old id and the server returns the new one, which the harness
				// stores as the computed id.
				Config: renamedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "id", acceptanceName+"-renamed"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "name", acceptanceName+"-renamed"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "model", "gpt-4o-mini"),
				),
			},
		},
	})
}

// TestAccLLMProviderReferenceResource covers the second provider option: a
// provider entry that references another entry by name.
func TestAccLLMProviderReferenceResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkProviderDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: referenceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.base", "id", acceptanceName+"-base"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "provider_reference", acceptanceName+"-base"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_provider.test",
				ImportState:       true,
				ImportStateId:     acceptanceName + "-reference",
				ImportStateVerify: true,
			},
		},
	})
}

const builtinConfig = `
resource "agentgateway_llm_provider" "test" {
  name     = "` + acceptanceName + `"
  provider_name = "openAI"
  model    = "gpt-4o"
  api_key  = "$ACCTEST_LLM_PROVIDER_KEY"
  base_url = "https://api.openai.com"
}
`

const updatedConfig = `
resource "agentgateway_llm_provider" "test" {
  name        = "` + acceptanceName + `"
  provider_name    = "openAI"
  model       = "gpt-4o-mini"
  api_key     = "$ACCTEST_LLM_PROVIDER_KEY"
  base_url    = "https://api.openai.com"
  path_prefix = "/openai"
}
`

const renamedConfig = `
resource "agentgateway_llm_provider" "test" {
  name          = "` + acceptanceName + `-renamed"
  provider_name = "openAI"
  model         = "gpt-4o-mini"
  api_key       = "$ACCTEST_LLM_PROVIDER_KEY"
  base_url      = "https://api.openai.com"
  path_prefix   = "/openai"
}
`

const referenceConfig = `
resource "agentgateway_llm_provider" "base" {
  name     = "` + acceptanceName + `-base"
  provider_name = "openAI"
  model    = "gpt-4o"
}

resource "agentgateway_llm_provider" "test" {
  name               = "` + acceptanceName + `-reference"
  provider_reference = agentgateway_llm_provider.base.name
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

func checkProviderDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindLLMProvider)
		if err != nil {
			return fmt.Errorf("listing providers: %w", err)
		}
		for _, res := range list {
			if res.ID == acceptanceName || res.ID == acceptanceName+"-base" || res.ID == acceptanceName+"-reference" || res.ID == acceptanceName+"-renamed" {
				return fmt.Errorf("provider %q still exists after destroy", res.ID)
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
	return []func() frameworkresource.Resource{New}
}
