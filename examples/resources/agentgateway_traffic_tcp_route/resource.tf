# A TCP route attached to a TCP gateway. The gateway's TCP backend host is a
# single "host:port" string, so the raw json variant carries it verbatim.
resource "agentgateway_traffic_tcp_route" "postgres" {
  name      = "postgres-backend"
  gateways  = ["postgres-proxy"]
  namespace = "default"
  rule_name = "postgres"

  hostnames = ["db.example.com"]

  backends {
    json = jsonencode({
      host = "postgres.example.com:5432"
    })
  }

  # TCP-level policies. A TCP route accepts backendTLS only; HTTP policies such
  # as cors and authorization are not available here.
  policies_json = jsonencode({
    backendTLS = {
      hostname = "postgres.example.com"
    }
  })
}

# Weighted backends: 80% of new connections go to the primary database and 20%
# to the replica. The service name is the `<namespace>/<hostname>` string the
# gateway resolves from its top-level services list.
resource "agentgateway_traffic_tcp_route" "replicated" {
  name     = "postgres-replicated"
  gateways = ["postgres-proxy"]

  backends {
    weight = 80

    service_json = jsonencode({
      name = "default/postgres-primary"
      port = 5432
    })
  }

  backends {
    weight = 20

    service_json = jsonencode({
      name = "default/postgres-replica"
      port = 5432
    })
  }
}
