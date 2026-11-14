# Every reusable provider entry, sorted by id.
data "agentgateway_llm_providers" "all" {}

# Only the entries whose id starts with a prefix.
data "agentgateway_llm_providers" "team" {
  name_prefix = "team-"
}

output "provider_names" {
  value = [for provider in data.agentgateway_llm_providers.all.providers : provider.name]
}
