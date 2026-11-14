# A route served by the default gateway. Requests to /api that carry the
# x-tenant header and a version query parameter are forwarded to a local
# service.
resource "agentgateway_traffic_route" "api" {
  name      = "api"
  gateways  = ["default"]
  namespace = "default"
  rule_name = "api"

  hostnames = ["api.example.com"]

  matches {
    path {
      prefix = "/api"
    }
    method = "GET"

    headers {
      name  = "x-tenant"
      exact = "acme"
    }

    query {
      name  = "version"
      regex = "^v[0-9]+$"
    }
  }

  backends {
    weight = 1

    host {
      host = "127.0.0.1"
      port = 8080
    }
  }

  policies_json = jsonencode({
    cors = {
      allowOrigins = ["https://example.com"]
    }
  })
}

# Weighted backends: 80% of the traffic goes to the stable service and 20% to
# the canary. Both use the service JSON form the gateway understands.
resource "agentgateway_traffic_route" "canary" {
  name      = "canary"
  gateways  = ["default"]
  hostnames = ["canary.example.com"]

  matches {
    path {
      exact = "/"
    }
  }

  backends {
    weight = 80

    service_json = jsonencode({
      name      = "stable"
      namespace = "default"
      port      = 8080
    })
  }

  backends {
    weight = 20

    service_json = jsonencode({
      name      = "canary"
      namespace = "default"
      port      = 8080
    })
  }
}
