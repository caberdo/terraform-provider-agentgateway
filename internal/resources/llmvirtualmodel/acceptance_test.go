package llmvirtualmodel

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

const acceptanceName = "acctest-virtual-model"

// TestAccVirtualModelResource covers create, import, update and destroy against
// the hybrid gateway. It is compiled by `go test` and only runs when TF_ACC is
// set and the acceptance environment is reachable.
func TestAccVirtualModelResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkVirtualModelDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: weightedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "id", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "name", acceptanceName),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.0.model", "acctest-model-a"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.0.weight", "90"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.1.model", "acctest-model-b"),
				),
			},
			{
				// Strict on purpose: if the real gateway omits weight/priority
				// from its response, this step must surface it, not mask it.
				ResourceName:      "agentgateway_llm_virtual_model.test",
				ImportState:       true,
				ImportStateId:     acceptanceName,
				ImportStateVerify: true,
			},
			{
				Config: failoverConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "failover.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "failover.0.targets.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "failover.0.targets.0.priority", "0"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "failover.0.targets.1.priority", "1"),
				),
			},
		},
	})
}

const weightedConfig = `
resource "agentgateway_llm_virtual_model" "test" {
  name = "` + acceptanceName + `"

  weighted {
    targets {
      model  = "acctest-model-a"
      weight = 90
    }

    targets {
      model = "acctest-model-b"
    }
  }
}
`

const failoverConfig = `
resource "agentgateway_llm_virtual_model" "test" {
  name = "` + acceptanceName + `"

  failover {
    targets {
      model    = "acctest-model-a"
      priority = 0
    }

    targets {
      model    = "acctest-model-b"
      priority = 1
    }
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

func checkVirtualModelDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindLLMVirtualModel)
		if err != nil {
			return fmt.Errorf("listing virtual models: %w", err)
		}
		for _, res := range list {
			if res.ID == acceptanceName {
				return fmt.Errorf("virtual model %q still exists after destroy", acceptanceName)
			}
		}
		return nil
	}
}

// testProvider is the smallest provider that can host this resource in
// an acceptance test.
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
