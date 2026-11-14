# List the llm.provider resources whose id starts with "shared-".
data "agentgateway_config_resources" "providers" {
  kind      = "llm.provider"
  id_prefix = "shared-"
}

# List every virtual API key. value_json is sensitive, so it is not exported.
data "agentgateway_config_resources" "api_keys" {
  kind = "llm.apiKey"
}

output "provider_ids" {
  value = [for resource in data.agentgateway_config_resources.providers.resources : resource.id]
}

output "api_key_revisions" {
  value = {
    for resource in data.agentgateway_config_resources.api_keys.resources : resource.id => resource.revision
  }
}
