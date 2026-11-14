# LLM policies are keyed by name: the name is the resource id and changing it
# replaces the policy (destroy, then create). A replacement creates a gap, so
# add `lifecycle { create_before_destroy = true }` when the gateway must never
# run without the policy.
#
# `spec` is the policy body, stored verbatim by the gateway. It may be any JSON
# value: most policies are objects, localRateLimit is an array of rules.

# CORS for the LLM listener.
resource "agentgateway_llm_policy" "cors" {
  name = "cors"

  spec = jsonencode({
    allowOrigins = ["https://app.example.com"]
    allowHeaders = ["authorization", "content-type"]
    allowMethods = ["GET", "POST"]
    maxAge       = "1h"
  })
}

# A local rate limit is an array, one entry per rule. This is why `spec` is not
# modelled as an object.
resource "agentgateway_llm_policy" "local_rate_limit" {
  name = "localRateLimit"

  spec = jsonencode([
    {
      maxTokens     = 100
      tokensPerFill = 100
      fillInterval  = "1s"
    },
  ])
}

# The apiKey policy enables API-key authentication. Keys themselves are managed
# by agentgateway_llm_api_key resources, so an inline `keys` list is rejected
# with a diagnostic instead of being written to the gateway.
resource "agentgateway_llm_policy" "api_key" {
  name = "apiKey"

  spec = jsonencode({
    mode = "strict"
  })
}

# A request timeout, expressed as a duration.
resource "agentgateway_llm_policy" "timeout" {
  name = "timeout"

  spec = jsonencode({
    requestTimeout = "30s"
  })
}
