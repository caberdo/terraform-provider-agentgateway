# A gateway with a server-side TLS certificate. Routes attach to it by name.
resource "agentgateway_traffic_gateway" "public" {
  name         = "public"
  port         = 8443
  bind_address = "0.0.0.0"
  protocol     = "HTTPS"

  tls_json = jsonencode({
    cert = file("${path.module}/server.crt")
    key  = file("${path.module}/server.key")
  })
}

# A gateway with named listeners. When listeners is set, only port and
# bind_address may be configured at the top level; TLS and policies move to
# the individual listeners.
resource "agentgateway_traffic_gateway" "multi_listener" {
  name         = "multi-listener"
  port         = 8080
  bind_address = "0.0.0.0"

  listeners {
    name     = "http"
    hostname = "example.com"
    protocol = "HTTP"
  }

  listeners {
    name     = "https"
    hostname = "secure.example.com"
    protocol = "HTTPS"

    tls_json = jsonencode({
      cert = file("${path.module}/secure.crt")
      key  = file("${path.module}/secure.key")
    })
  }
}
