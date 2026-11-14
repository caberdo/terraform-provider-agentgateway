# UI policies apply to the agentgateway web UI. The policy key is the resource
# id, so changing `name` replaces the policy (destroy, then create); add
# `lifecycle { create_before_destroy = true }` when the UI must never run
# without the policy.
#
# `spec` is the policy body, stored verbatim by the gateway. Any JSON value is
# accepted; the documented bodies are objects.

# Allow the UI to be called from a browser app on another origin.
resource "agentgateway_ui_policy" "cors" {
  name = "cors"

  spec = jsonencode({
    allowOrigins = ["https://app.example.com"]
    allowHeaders = ["authorization", "content-type"]
    allowMethods = ["GET", "POST"]
  })
}

# Browser login for the UI through an OIDC provider.
resource "agentgateway_ui_policy" "oidc" {
  name = "oidc"

  spec = jsonencode({
    issuer       = "https://auth.example.com"
    clientId     = "agentgateway-ui"
    redirectPath = "/oauth2/callback"
  })
}
