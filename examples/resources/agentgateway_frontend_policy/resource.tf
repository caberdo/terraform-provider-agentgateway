# Frontend policies apply to the frontend listener, before any traffic route
# is selected. The policy key is the resource id, so changing `name` replaces
# the policy (destroy, then create); add
# `lifecycle { create_before_destroy = true }` when the listener must never run
# without the policy.
#
# `spec` is the policy body, stored verbatim by the gateway. Any JSON value is
# accepted; the documented bodies are objects.

# Structured access logging for the frontend listener.
resource "agentgateway_frontend_policy" "access_log" {
  name = "accessLog"

  spec = jsonencode({
    format = "json"
    filter = "status >= 400"
  })
}

# TLS settings for the frontend listener.
resource "agentgateway_frontend_policy" "tls" {
  name = "tls"

  spec = jsonencode({
    minVersion = "1.2"
    maxVersion = "1.3"
  })
}

# HTTP-level limits applied before routing.
resource "agentgateway_frontend_policy" "http" {
  name = "http"

  spec = jsonencode({
    maxRequestBodyBytes = 10485760
  })
}
