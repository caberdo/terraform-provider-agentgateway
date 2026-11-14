# Every route the agentgateway currently holds, sorted by id.
data "agentgateway_traffic_routes" "all" {}

# Narrow the listing to routes whose name starts with "api-".
data "agentgateway_traffic_routes" "api" {
  name_prefix = "api-"
}

output "route_hostnames" {
  value = {
    for route in data.agentgateway_traffic_routes.all.routes : route.name => route.hostnames
  }
}

# Backends are raw JSON objects; decode them to inspect a backend field.
output "api_route_backends" {
  value = [
    for route in data.agentgateway_traffic_routes.api.routes : [
      for backend in route.backends : jsondecode(backend)
    ]
  ]
}
