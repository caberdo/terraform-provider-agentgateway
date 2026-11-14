# The merged configuration the gateway is currently serving.
data "agentgateway_effective_config" "current" {}

# The whole document is sensitive because it can contain provider credentials
# and API key material, so the output must be marked sensitive as well.
output "effective_config" {
  value     = data.agentgateway_effective_config.current.json
  sensitive = true
}
