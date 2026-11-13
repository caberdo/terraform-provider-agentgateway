package llmprovider

import (
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLLMProviderCRUD drives the resource through Terraform against the
// protocol fake: create, import and update (rename plus new parameters).
func TestLLMProviderCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkProviderDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_provider" "test" {
  name     = "openai-prod"
  provider_name = "openAI"
  model    = "gpt-4o"
  api_key  = "$ACCTEST_KEY"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "id", "openai-prod"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "revision", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "provider_name", "openAI"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "model", "gpt-4o"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "api_key", "$ACCTEST_KEY"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_provider.test", "base_url"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_provider.test",
				ImportState:       true,
				ImportStateId:     "openai-prod",
				ImportStateVerify: true,
			},
			{
				// Rename is an in-place update on the server (spec C7), but the
				// protocol fake does not implement rename_prepared: it keeps the
				// id it was given. The acceptance test covers the rename against
				// a real gateway; here we pin the parameter update path.
				Config: `
resource "agentgateway_llm_provider" "test" {
  name        = "openai-prod"
  provider_name = "openAI"
  model       = "gpt-4o-mini"
  api_key     = "$ACCTEST_KEY"
  base_url    = "https://api.openai.com"
  path_prefix = "/openai"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "id", "openai-prod"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "revision", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "model", "gpt-4o-mini"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "base_url", "https://api.openai.com"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "path_prefix", "/openai"),
				),
			},
		},
	})
}

// TestLLMProviderReferenceCRUD covers the reference and custom provider
// options end to end.
func TestLLMProviderReferenceCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkProviderDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_provider" "base" {
  name     = "acctest-llm-provider-base"
  provider_name = "openAI"
}

resource "agentgateway_llm_provider" "test" {
  name               = "acctest-llm-provider-reference"
  provider_reference = agentgateway_llm_provider.base.name
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "id", "acctest-llm-provider-reference"),
					resource.TestCheckResourceAttr("agentgateway_llm_provider.test", "provider_reference", "acctest-llm-provider-base"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_provider.test", "provider_name"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_provider.test",
				ImportState:       true,
				ImportStateId:     "acctest-llm-provider-reference",
				ImportStateVerify: true,
			},
		},
	})
}

// TestLLMProviderSchemaRequiresOneProviderOption pins the declarative half of
// the exactly-one-of rule: zero options, two options and an unknown provider
// name are rejected at plan time, before any request reaches the gateway.
func TestLLMProviderSchemaRequiresOneProviderOption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "zero provider options",
			config: `
resource "agentgateway_llm_provider" "test" {
  name = "p"
}`,
			wantErr: `one \(and only one\)`,
		},
		{
			name: "two provider options",
			config: `
resource "agentgateway_llm_provider" "test" {
  name     = "p"
  provider_name = "openAI"
  model    = "gpt-4o"

  provider_reference = "other"
}`,
			wantErr: `one \(and only one\)`,
		},
		{
			name: "provider and custom provider",
			config: `
resource "agentgateway_llm_provider" "test" {
  name                 = "p"
  provider_name             = "openAI"
  custom_provider_json = jsonencode({ model = "llama3" })
}`,
			wantErr: `one \(and only one\)`,
		},
		{
			name: "unknown provider name",
			config: `
resource "agentgateway_llm_provider" "test" {
  name     = "p"
  provider_name = "not-a-provider"
}`,
			wantErr: `value must be one of`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, _ := agentgatewaytest.New(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testFactories(server.URL),
				Steps: []resource.TestStep{
					{
						Config:      tt.config,
						ExpectError: regexp.MustCompile(tt.wantErr),
					},
				},
			})
		})
	}
}
