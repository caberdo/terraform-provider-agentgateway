package effectiveconfig

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

// TestAccEffectiveConfigDataSource creates a provider in the same test so the
// effective document is non-empty, then reads it back through the data source.
// It is compiled by `go test` and only runs when TF_ACC is set and the
// acceptance environment is reachable.
//
// The name carries a per-run suffix: the acceptance gateway is shared, so a
// stale provider left by a failed run must not interfere with this test.
func TestAccEffectiveConfigDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	suffix := time.Now().UnixNano()
	providerName := fmt.Sprintf("acctest-effective-config-%d", suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkProviderDestroyed(endpoint, providerName),
		Steps: []resource.TestStep{
			{
				Config: effectiveConfigConfig(providerName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("data.agentgateway_effective_config.test", "json", func(value string) error {
						if value == "" {
							return fmt.Errorf("effective config is empty")
						}
						if !json.Valid([]byte(value)) {
							return fmt.Errorf("effective config is not valid JSON")
						}
						return nil
					}),
				),
			},
		},
	})
}

// effectiveConfigConfig builds the acceptance configuration for the backing
// provider and the data source.
func effectiveConfigConfig(providerName string) string {
	return fmt.Sprintf(`
resource "agentgateway_test_resource" "provider" {
  name = %q

  value_json = jsonencode({
    name     = %q
    provider = "openai"
    model    = "gpt-4o-mini"
  })
}

data "agentgateway_effective_config" "test" {
  depends_on = [agentgateway_test_resource.provider]
}
`, providerName, providerName)
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
