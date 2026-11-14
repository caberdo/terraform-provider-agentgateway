package effectiveconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const accGatewayName = "acctest-effective-config"

// TestAccEffectiveConfigDataSource creates a gateway in the same test so the
// effective document is non-empty, then reads it back through the data source.
// It is compiled by `go test` and only runs when TF_ACC is set and the
// acceptance environment is reachable.
func TestAccEffectiveConfigDataSource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkGatewayDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_test_resource" "gateway" {
  name = "` + accGatewayName + `"

  value_json = jsonencode({
    name        = "` + accGatewayName + `"
    port        = 18091
    bindAddress = "0.0.0.0"
    protocol    = "HTTP"
  })
}

data "agentgateway_effective_config" "test" {
  depends_on = [agentgateway_test_resource.gateway]
}
`,
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

// checkGatewayDestroyed proves the backing gateway is gone after destroy.
func checkGatewayDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindTrafficGateway)
		if err != nil {
			return fmt.Errorf("listing traffic gateways: %w", err)
		}
		for _, res := range list {
			if res.ID == accGatewayName {
				return fmt.Errorf("traffic gateway %q still exists after destroy", res.ID)
			}
		}
		return nil
	}
}
