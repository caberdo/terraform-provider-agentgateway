package llmvirtualmodel

import (
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestVirtualModelCRUD drives the resource through Terraform against the
// protocol fake: create, import and update (which switches routing mode).
func TestVirtualModelCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkVirtualModelDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_llm_virtual_model" "test" {
  name = "vm"

  weighted {
    targets {
      model  = "a"
      weight = 90
    }

    targets {
      model = "b"
    }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "id", "vm"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "revision", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.0.weight", "90"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_virtual_model.test", "weighted.0.targets.1.weight"),
				),
			},
			{
				ResourceName:      "agentgateway_llm_virtual_model.test",
				ImportState:       true,
				ImportStateId:     "vm",
				ImportStateVerify: true,
			},
			{
				Config: `
resource "agentgateway_llm_virtual_model" "test" {
  name = "vm"

  failover {
    targets {
      model    = "a"
      priority = 0
    }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "failover.#", "1"),
					resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "failover.0.targets.0.priority", "0"),
					resource.TestCheckNoResourceAttr("agentgateway_llm_virtual_model.test", "weighted.#"),
				),
			},
		},
	})
}

// TestVirtualModelSchemaRequiresRouting pins the declarative half of the
// exactly-one-of rule: zero routing blocks and two routing blocks are rejected
// at plan time, before any request reaches the gateway.
func TestVirtualModelSchemaRequiresRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config string
	}{
		{
			name:   "zero routing blocks",
			config: `resource "agentgateway_llm_virtual_model" "test" { name = "vm" }`,
		},
		{
			name: "two routing blocks",
			config: `
resource "agentgateway_llm_virtual_model" "test" {
  name = "vm"

  weighted {
    targets { model = "a" }
  }

  failover {
    targets { model = "b" }
  }
}`,
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
						ExpectError: regexp.MustCompile(`one \(and only one\)`),
					},
				},
			})
		})
	}
}

// TestVirtualModelSchemaConditionalWhen pins that the schema accepts a final
// conditional target without when and that Value rejects an earlier one.
func TestVirtualModelSchemaConditionalWhen(t *testing.T) {
	t.Parallel()

	t.Run("non-final target without when is rejected", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_virtual_model" "test" {
  name = "vm"

  conditional {
    targets { model = "a" }
    targets { model = "b" }
  }
}`,
					ExpectError: regexp.MustCompile(`conditional target 0 requires when`),
				},
			},
		})
	})

	t.Run("final target without when is accepted", func(t *testing.T) {
		t.Parallel()

		server, _ := agentgatewaytest.New(t)
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: testFactories(server.URL),
			CheckDestroy:             checkVirtualModelDestroyed(server.URL),
			Steps: []resource.TestStep{
				{
					Config: `
resource "agentgateway_llm_virtual_model" "test" {
  name = "vm"

  conditional {
    targets {
      model = "a"
      when  = "request.headers[\"x-tier\"] == \"free\""
    }

    targets { model = "b" }
  }
}`,
					Check: resource.TestCheckResourceAttr("agentgateway_llm_virtual_model.test", "conditional.0.targets.#", "2"),
				},
			},
		})
	})
}
