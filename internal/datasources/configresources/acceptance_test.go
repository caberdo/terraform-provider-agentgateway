package configresources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccConfigResourcesDataSource creates a provider in the same test and
// reads it back through the generic kind listing. It is compiled by `go test`
// and only runs when TF_ACC is set and the acceptance environment is
// reachable.
//
// Names carry a per-run suffix: the acceptance gateway is shared, so a stale
// provider left by a failed run must not change this test's counts.
func TestAccConfigResourcesDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	suffix := time.Now().UnixNano()
	prefix := fmt.Sprintf("acctest-config-%d", suffix)
	providerName := prefix + "-provider"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkProviderDestroyed(endpoint, providerName),
		Steps: []resource.TestStep{
			{
				Config: configResourcesConfig(prefix, providerName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "id", "llm.provider:"+prefix),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.#", "1"),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.0.id", providerName),
					resource.TestCheckResourceAttr("data.agentgateway_config_resources.test", "resources.0.revision", "1"),
					resource.TestCheckResourceAttrSet("data.agentgateway_config_resources.test", "resources.0.created_at"),
					resource.TestCheckResourceAttrSet("data.agentgateway_config_resources.test", "resources.0.updated_at"),
					resource.TestCheckResourceAttrWith("data.agentgateway_config_resources.test", "resources.0.value_json", func(value string) error {
						var decoded map[string]any
						return json.Unmarshal([]byte(value), &decoded)
					}),
				),
			},
		},
	})
}

// configResourcesConfig builds the acceptance configuration for the provider
// and the listing.
func configResourcesConfig(prefix, providerName string) string {
	return fmt.Sprintf(`
resource "agentgateway_test_resource" "provider" {
  name = %q

  value_json = jsonencode({
    name     = %q
    provider = "openai"
    model    = "gpt-4o-mini"
  })
}

data "agentgateway_config_resources" "test" {
  kind      = "llm.provider"
  id_prefix = %q

  depends_on = [agentgateway_test_resource.provider]
}
`, providerName, providerName, prefix)
}

// checkProviderDestroyed proves the backing provider is gone after destroy.
func checkProviderDestroyed(endpoint string, names ...string) resource.TestCheckFunc {
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
			return fmt.Errorf("listing llm providers: %w", err)
		}
		for _, res := range list {
			for _, name := range names {
				if res.ID == name {
					return fmt.Errorf("llm provider %q still exists after destroy", res.ID)
				}
			}
		}
		return nil
	}
}
