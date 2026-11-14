package llmapikey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/acctest"
	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
	"github.com/caberdo/terraform-provider-agentgateway/internal/providerdata"
	"github.com/caberdo/terraform-provider-agentgateway/internal/resourceutil"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// acceptanceTeam marks the keys the acceptance suite creates, so CheckDestroy
// can recognise them without knowing the server-generated UUID.
const acceptanceTeam = "acctest-l8"

// TestAccLLMAPIKeyResource covers create, import, update and destroy of a
// plaintext key against the hybrid gateway. It is compiled by `go test` and
// only runs when TF_ACC is set and the acceptance environment is reachable.
//
// The config includes the apiKey policy because a key cannot be created without
// it (spec C10); the local test provider registers that policy resource from
// the shared harness, so this package still depends on no other lane.
func TestAccLLMAPIKeyResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkAPIKeyDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: apiKeyConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("agentgateway_llm_api_key.test", "id"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "key", acceptanceKey),
					resource.TestCheckNoResourceAttr("agentgateway_llm_api_key.test", "key_hash"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "metadata.team", acceptanceTeam),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "allowed_models.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.0.name", "daily-spend"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.0.limit.unit", "USD"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.0.limit.amount", "50"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.0.window.rolling", "24h"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "budgets.0.on_budget_exceeded", "Block"),
				),
			},
			{
				// The id is a server-generated UUID, so it comes from state.
				// The key material and the computed timestamps are not
				// guaranteed to be echoed back by the gateway, hence the ignore
				// list and the non-empty plan after import.
				ResourceName:            "agentgateway_llm_api_key.test",
				ImportState:             true,
				ImportStateIdFunc:       apiKeyImportID,
				ImportStateVerify:       true,
				ExpectNonEmptyPlan:      true,
				ImportStateVerifyIgnore: []string{"key", "key_hash", "created_at", "updated_at"},
			},
			{
				Config: updatedAPIKeyConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "key", acceptanceKey+"-rotated"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "metadata.team", acceptanceTeam),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "allowed_models.#", "0"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_api_key.test", "budgets"),
				),
			},
		},
	})
}

// TestAccLLMAPIKeyHashedResource covers the second key variant: a sha256 hash
// instead of a plaintext key.
func TestAccLLMAPIKeyHashedResource(t *testing.T) {
	acctest.PreCheck(t)
	endpoint := acctest.EndpointFor(t, acctest.RoleHybrid)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(endpoint),
		CheckDestroy:             checkAPIKeyDestroyed(endpoint),
		Steps: []resource.TestStep{
			{
				Config: hashedAPIKeyConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("agentgateway_llm_api_key.test", "id"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "key_hash", apiKeyHash),
					resource.TestCheckNoResourceAttr("agentgateway_llm_api_key.test", "key"),
					resource.TestCheckResourceAttr("agentgateway_llm_api_key.test", "metadata.team", acceptanceTeam),
				),
			},
			{
				ResourceName:            "agentgateway_llm_api_key.test",
				ImportState:             true,
				ImportStateIdFunc:       apiKeyImportID,
				ImportStateVerify:       true,
				ExpectNonEmptyPlan:      true,
				ImportStateVerifyIgnore: []string{"key", "key_hash", "created_at", "updated_at"},
			},
		},
	})
}

// apiKeyImportID reads the server-generated id from state.
func apiKeyImportID(state *terraform.State) (string, error) {
	instance, ok := state.RootModule().Resources["agentgateway_llm_api_key.test"]
	if !ok {
		return "", fmt.Errorf("agentgateway_llm_api_key.test is not in state")
	}
	return instance.Primary.Attributes["id"], nil
}

const acceptanceKey = "sk-acctest-l8-0001"

const apiKeyConfig = `
resource "agentgateway_llm_policy" "api_key" {
  name = "apiKey"
  spec = jsonencode({ mode = "strict" })
}

resource "agentgateway_llm_api_key" "test" {
  key      = "` + acceptanceKey + `"
  metadata = { team = "` + acceptanceTeam + `", name = "` + acceptanceTeam + `" }
  allowed_models = ["gpt-4o-mini", "openai/*"]

  # The gateway rejects a key with budgets when metadata.name is missing.
  depends_on = [agentgateway_llm_policy.api_key]

  budgets = [
    {
      name               = "daily-spend"
      limit              = { unit = "USD", amount = 50 }
      window             = { rolling = "24h" }
      on_budget_exceeded = "Block"
    },
  ]
}
`

const updatedAPIKeyConfig = `
resource "agentgateway_llm_policy" "api_key" {
  name = "apiKey"
  spec = jsonencode({ mode = "strict" })
}

resource "agentgateway_llm_api_key" "test" {
  key            = "` + acceptanceKey + `-rotated"
  metadata       = { team = "` + acceptanceTeam + `", name = "` + acceptanceTeam + `" }
  allowed_models = []

  depends_on = [agentgateway_llm_policy.api_key]
}
`

const hashedAPIKeyConfig = `
resource "agentgateway_llm_policy" "api_key" {
  name = "apiKey"
  spec = jsonencode({ mode = "strict" })
}

resource "agentgateway_llm_api_key" "test" {
  key_hash = "` + apiKeyHash + `"
  metadata = { team = "` + acceptanceTeam + `", name = "` + acceptanceTeam + `" }

  # Terraform would otherwise create the key and its prerequisite policy in
  # parallel, and the gateway rejects a key before the policy exists (C10).
  depends_on = [agentgateway_llm_policy.api_key]
}
`

// testFactories builds a provider that registers this resource, plus the apiKey
// policy it needs at runtime. The resource package must not import
// internal/provider (design §4), so the acceptance test supplies the minimal
// provider it needs. The policy resource comes from the shared harness; the
// policy lane's package is not imported.
func testFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	p := &testProvider{endpoint: endpoint}
	return map[string]func() (tfprotov6.ProviderServer, error){
		"agentgateway": providerserver.NewProtocol6WithError(p),
	}
}

func checkAPIKeyDestroyed(endpoint string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c, err := client.New(client.Config{
			BaseURL:    endpoint,
			HTTPClient: http.DefaultClient,
			UserAgent:  "terraform-provider-agentgateway/acctest",
		})
		if err != nil {
			return fmt.Errorf("creating acceptance client: %w", err)
		}

		list, err := c.List(context.Background(), configresource.KindLLMAPIKey)
		if err != nil {
			return fmt.Errorf("listing api keys: %w", err)
		}
		for _, res := range list {
			var value struct {
				Metadata map[string]string `json:"metadata"`
			}
			if err := json.Unmarshal(res.Value, &value); err != nil {
				continue
			}
			if value.Metadata["team"] == acceptanceTeam {
				return fmt.Errorf("api key %q still exists after destroy", res.ID)
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
	return []func() frameworkresource.Resource{
		New,
		// The apiKey policy is the prerequisite of every llm.apiKey write
		// (spec C10). It is registered from the shared harness rather than
		// imported from the policy lane's package.
		func() frameworkresource.Resource {
			return resourceutil.CRUD(resourceutil.Spec{
				TypeName: "agentgateway_llm_policy",
				Kind:     configresource.KindLLMPolicy,
				Schema:   resourceutil.PolicySchema(configresource.LLMPolicyKeys),
				New: func() resourceutil.KindResource {
					return resourceutil.NewPolicyModel(configresource.KindLLMPolicy)
				},
			})
		},
	}
}
