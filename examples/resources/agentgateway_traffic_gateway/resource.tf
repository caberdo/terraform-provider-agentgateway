# Placeholder PEM material so this example validates without certificate
# files. Replace the values with real certificates (or file() references to
# them) before applying the configuration.
locals {
  server_cert = "-----BEGIN CERTIFICATE-----\nplaceholder-server-certificate\n-----END CERTIFICATE-----"
  server_key  = "-----BEGIN PRIVATE KEY-----\nplaceholder-server-key\n-----END PRIVATE KEY-----"
  secure_cert = "-----BEGIN CERTIFICATE-----\nplaceholder-secure-certificate\n-----END CERTIFICATE-----"
  secure_key  = "-----BEGIN PRIVATE KEY-----\nplaceholder-secure-key\n-----END PRIVATE KEY-----"
}

# A gateway with a server-side TLS certificate. Routes attach to it by name.
resource "agentgateway_traffic_gateway" "public" {
  name         = "public"
  port         = 8443
  bind_address = "0.0.0.0"
  protocol     = "HTTPS"

  tls_json = jsonencode({
    cert = local.server_cert
    key  = local.server_key
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
      cert = local.secure_cert
      key  = local.secure_key
    })
  }
}
