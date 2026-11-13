package llmmodel

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

const acceptanceName = "acctest-llm-model"

// TestAccLLMModelResource covers create, import, update and destroy against the
// hybrid gateway. It is compiled by `go test` and only runs when TF_ACC is set
// and the acceptance environment is reachable.
func TestAccLLMModelResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkLLMModelDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: modelConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "id", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "name", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "provider_name", "openai"),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "model", "gpt-4o-mini"),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "visibility", "public"),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "display_name", "Acceptance Model"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_model.test",
				ImportState:       true,
				ImportStateId:     acceptanceName,
				ImportStateVerify: true,
				// api_key is sensitive and the gateway expands the env reference
				// on read. JSON leaves the configuration set are preserved as
				// null by Read (see preserveJSONLeaf), so import cannot verify
				// them either.
				ImportStateVerifyIgnore: []string{"api_key", "defaults_json"},
			},
			{
				Config: modelUpdatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "id", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "model", "gpt-4o"),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "visibility", "internal"),
					resource.TestCheckResourceAttr("agentgateway_llm_model.test", "display_name", "Acceptance Model v2"),
				),
			},
		},
	})
}

const modelConfig = `
resource "agentgateway_llm_model" "test" {
  name         = "` + acceptanceName + `"
  provider_name = "openai"
  model        = "gpt-4o-mini"
  display_name = "Acceptance Model"
  api_key      = "$ACCTEST_OPENAI_API_KEY"

  defaults_json = jsonencode({ temperature = 0.2 })
}
`

const modelUpdatedConfig = `
resource "agentgateway_llm_model" "test" {
  name         = "` + acceptanceName + `"
  provider_name = "openai"
  model        = "gpt-4o"
  display_name = "Acceptance Model v2"
  visibility   = "internal"
  api_key      = "$ACCTEST_OPENAI_API_KEY"

  defaults_json = jsonencode({ temperature = 0.2 })
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

func checkLLMModelDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindLLMModel)
		if err != nil {
			return fmt.Errorf("listing llm models: %w", err)
		}
		for _, res := range list {
			if res.ID == acceptanceName {
				return fmt.Errorf("llm model %q still exists after destroy", acceptanceName)
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
