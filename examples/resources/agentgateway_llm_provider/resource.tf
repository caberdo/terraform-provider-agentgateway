# Built-in provider with a model and an environment-referenced credential.
# The gateway resolves "$OPENAI_API_KEY" at runtime; the key never lands in
# Terraform state.
resource "agentgateway_llm_provider" "openai" {
  name          = "openai-prod"
  provider_name = "openAI"
  model         = "gpt-4o"
  api_key       = "$OPENAI_API_KEY"
  base_url      = "https://api.openai.com"
}

# Preset provider. Presets carry their own upstream defaults; only the
# credential and the region-specific parameters are set here.
resource "agentgateway_llm_provider" "deepseek" {
  name          = "deepseek-eu"
  provider_name = "deepseek"
  model         = "deepseek-chat"
  api_key       = "$DEEPSEEK_API_KEY"
}

# A provider that inherits another entry by name. Models can reference either
# entry; this one only overrides the model.
resource "agentgateway_llm_provider" "openai_mini" {
  name               = "openai-mini"
  provider_reference = agentgateway_llm_provider.openai.name
  model              = "gpt-4o-mini"
}

# A self-hosted provider, expressed as JSON because the custom provider shape
# is an escape hatch for anything the typed attributes do not cover.
resource "agentgateway_llm_provider" "self_hosted" {
  name = "self-hosted"

  custom_provider_json = jsonencode({
    model            = "llama3"
    providerOverride = "vllm"
    formats = [
      { type = "completions" },
    ]
  })

  base_url = "http://vllm.internal:8000"
}
