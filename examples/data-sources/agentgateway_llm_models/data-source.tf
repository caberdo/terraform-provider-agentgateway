# Every model the gateway serves, sorted by id.
data "agentgateway_llm_models" "all" {}

# Only the models whose id starts with a prefix. Model ids default to the name.
data "agentgateway_llm_models" "openai" {
  name_prefix = "openai-"
}

output "model_names" {
  value = [for model in data.agentgateway_llm_models.all.models : model.name]
}
