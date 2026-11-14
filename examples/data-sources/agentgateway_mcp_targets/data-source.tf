# Every MCP target, sorted by id.
data "agentgateway_mcp_targets" "all" {}

# Only the targets whose id (the name) starts with a prefix.
data "agentgateway_mcp_targets" "team" {
  name_prefix = "team-"
}

output "target_addresses" {
  value = [
    for target in data.agentgateway_mcp_targets.all.targets : {
      name      = target.name
      transport = target.transport
      host      = target.host
      path      = target.path
    }
  ]
}
