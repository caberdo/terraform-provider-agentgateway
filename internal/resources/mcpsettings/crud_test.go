package mcpsettings

import (
	"regexp"
	"testing"

	"github.com/caberdo/terraform-provider-agentgateway/internal/agentgatewaytest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestMCPSettingsCRUD drives the singleton through Terraform against the
// protocol fake: create, import and update from gateways to the deprecated
// port listener.
func TestMCPSettingsCRUD(t *testing.T) {
	server, _ := agentgatewaytest.New(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		CheckDestroy:             checkSettingsDestroyed(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_settings" "test" {
  gateways     = ["default", "mcp/listener"]
  stateful_mode = "stateless"
  prefix_mode   = "always"
  failure_mode  = "failClosed"
  sse_keep_alive = "30s"
  dns_rebinding_protection = true

  server = {
    name         = "agentgateway"
    version      = "1.7.0"
    title        = "Gateway"
    instructions = "Use tools."
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "id", "default"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "revision", "1"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "gateways.#", "2"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "gateways.0", "default"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "gateways.1", "mcp/listener"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "stateful_mode", "stateless"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "prefix_mode", "always"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "failure_mode", "failClosed"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "sse_keep_alive", "30s"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "dns_rebinding_protection", "true"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "server.name", "agentgateway"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "server.version", "1.7.0"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_settings.test", "port"),
				),
			},
			{
				ResourceName:      "agentgateway_mcp_settings.test",
				ImportState:       true,
				ImportStateId:     "default",
				ImportStateVerify: true,
				// A gateway release may materialize defaults, so the plan after
				// import need not be empty.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: `
resource "agentgateway_mcp_settings" "test" {
  port            = 8080
  prefix_mode     = "conditional"
  failure_mode    = "failOpen"
  sse_keep_alive  = "15s"
  dns_rebinding_protection = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "revision", "2"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "port", "8080"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "prefix_mode", "conditional"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "failure_mode", "failOpen"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "sse_keep_alive", "15s"),
					resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "dns_rebinding_protection", "false"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_settings.test", "gateways.#"),
					resource.TestCheckNoResourceAttr("agentgateway_mcp_settings.test", "server.name"),
				),
			},
		},
	})
}

// TestMCPSettingsPrefixModes pins that every `prefixMode` value the gateway
// documents is accepted: `conditional` and `always`, plus `never`, which
// upstream added for clients that call tools by their plain names.
func TestMCPSettingsPrefixModes(t *testing.T) {
	for _, mode := range []string{"conditional", "always", "never"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			server, _ := agentgatewaytest.New(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testFactories(server.URL),
				Steps: []resource.TestStep{
					{
						Config: `
resource "agentgateway_mcp_settings" "test" {
  prefix_mode = "` + mode + `"
}`,
						Check: resource.TestCheckResourceAttr("agentgateway_mcp_settings.test", "prefix_mode", mode),
					},
				},
			})
		})
	}
}

// TestMCPSettingsPlanTimeValidation pins the settings rules that must fail
// during `terraform plan`: port and gateways conflict upstream, and
// sse_keep_alive must be a Go duration.
func TestMCPSettingsPlanTimeValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "port and gateways",
			config: `
resource "agentgateway_mcp_settings" "test" {
  port     = 8080
  gateways = ["default"]
}`,
			wantErr: `cannot be specified when`,
		},
		{
			name: "sse keep alive without unit",
			config: `
resource "agentgateway_mcp_settings" "test" {
  sse_keep_alive = "30"
}`,
			wantErr: `Go duration`,
		},
		{
			name: "sse keep alive not a duration",
			config: `
resource "agentgateway_mcp_settings" "test" {
  sse_keep_alive = "often"
}`,
			wantErr: `Go duration`,
		},
		{
			name: "valid duration",
			config: `
resource "agentgateway_mcp_settings" "test" {
  sse_keep_alive = "1m30s"
}`,
		},
		{
			name: "valid gateways",
			config: `
resource "agentgateway_mcp_settings" "test" {
  gateways = ["default"]
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, _ := agentgatewaytest.New(t)
			step := resource.TestStep{
				Config:             tt.config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			}
			if tt.wantErr != "" {
				step.ExpectError = regexp.MustCompile(tt.wantErr)
			}

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testFactories(server.URL),
				Steps:                    []resource.TestStep{step},
			})
		})
	}
}

// TestMCPSettingsSchemaServerPair pins the server name/version rule before any
// request reaches the gateway.
func TestMCPSettingsSchemaServerPair(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactories(server.URL),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_settings" "test" {
  server = {
    name = "gateway-without-version"
  }
}`,
				ExpectError: regexp.MustCompile(`must be specified when`),
			},
		},
	})
}

// TestMCPSettingsRejectedInFileMode pins the singleton's storage-mode rule:
// file mode cannot read the overlay back, so the resource refuses up front
// instead of dropping itself from state on the next refresh.
func TestMCPSettingsRejectedInFileMode(t *testing.T) {
	t.Parallel()

	server, _ := agentgatewaytest.New(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testFactoriesWithStoreMode(server.URL, "file"),
		Steps: []resource.TestStep{
			{
				Config: `
resource "agentgateway_mcp_settings" "test" {
  gateways = ["default"]
}`,
				ExpectError: regexp.MustCompile(`requires config\.storage\.mode = hybrid`),
			},
		},
	})
}
