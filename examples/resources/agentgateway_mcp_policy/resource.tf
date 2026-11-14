# MCP policies are keyed by name: the name is the resource id and changing it
# replaces the policy (destroy, then create). A replacement creates a gap, so
# add `lifecycle { create_before_destroy = true }` when the gateway must never
# run without the policy.
#
# `spec` is the policy body, stored verbatim by the gateway. It may be any JSON
# value: most policies are objects, localRateLimit is an array of rules.

# Authenticate MCP clients with OIDC.
resource "agentgateway_mcp_policy" "oidc" {
  name = "oidc"

  spec = jsonencode({
    issuer       = "https://issuer.example.com"
    audience     = "agentgateway"
    clientId     = "mcp-gateway"
    clientSecret = "$MCP_OIDC_CLIENT_SECRET"
    scopes       = ["openid", "profile"]
  })
}

# Add a header to every MCP request before it leaves the gateway.
resource "agentgateway_mcp_policy" "request_header_modifier" {
  name = "requestHeaderModifier"

  spec = jsonencode({
    add = [
      {
        name  = "x-mcp-gateway"
        value = "terraform"
      },
    ]
  })
}

# A local rate limit is an array, one entry per rule. This is why `spec` is not
# modelled as an object.
resource "agentgateway_mcp_policy" "local_rate_limit" {
  name = "localRateLimit"

  spec = jsonencode([
    {
      maxTokens     = 50
      tokensPerFill = 50
      fillInterval  = "1s"
    },
  ])
}

# Answer directly without contacting an MCP backend.
resource "agentgateway_mcp_policy" "direct_response" {
  name = "directResponse"

  spec = jsonencode({
    status = 503
    body   = "MCP backend is unavailable"
  })
}
