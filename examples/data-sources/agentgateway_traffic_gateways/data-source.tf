# Every gateway the agentgateway currently holds, sorted by name.
data "agentgateway_traffic_gateways" "all" {}

# Narrow the listing to gateways whose name starts with "public-".
data "agentgateway_traffic_gateways" "public" {
  name_prefix = "public-"
}

output "gateway_names" {
  value = [for gateway in data.agentgateway_traffic_gateways.all.gateways : gateway.name]
}

output "public_listener_hostnames" {
  value = flatten([
    for gateway in data.agentgateway_traffic_gateways.public.gateways : [
      for listener in gateway.listeners : listener.hostname
    ]
  ])
}
