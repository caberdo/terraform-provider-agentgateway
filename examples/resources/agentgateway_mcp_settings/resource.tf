# The MCP settings singleton. The id is always `default`, so there is exactly
# one of these per gateway. It needs config.storage.mode = hybrid, because a
# file-mode gateway cannot read the overlay back.
resource "agentgateway_mcp_settings" "default" {
  gateways = ["default", "mcp/listener"]

  # How sessions and tool names behave when several targets are multiplexed.
  # prefix_mode is one of `always`, `conditional` (the gateway default) or
  # `never`; `never` requires tool names to be unique across targets.
  stateful_mode  = "stateless"
  prefix_mode    = "always"
  failure_mode   = "failClosed"
  sse_keep_alive = "30s"

  # Reject requests whose Host/Origin headers are not localhost. Leave this
  # unset to keep the gateway default.
  dns_rebinding_protection = true

  # The serverInfo and preamble clients see when the gateway multiplexes
  # targets. `name` and `version` must be set together.
  server = {
    name         = "agentgateway"
    version      = "1.7.0"
    title        = "Agent gateway"
    instructions = "Prefer the target-specific tools when they are available."
  }
}

# The deprecated alternative binds MCP traffic to a dedicated port. Prefer
# `gateways`; the gateway does not expect both forms at once:
#
# resource "agentgateway_mcp_settings" "default" {
#   port        = 3000
#   prefix_mode = "conditional"
# }
#
# A field the gateway config file also sets is file-owned. Writing a different
# value fails with a diagnostic that says the config file owns the field; remove
# it from the file and restart the gateway to manage it with Terraform.
