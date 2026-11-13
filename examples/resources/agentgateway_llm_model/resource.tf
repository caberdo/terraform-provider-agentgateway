# A public model that clients request directly. The provider is one of the
# names built into agentgateway; api_key accepts an environment reference.
resource "agentgateway_llm_model" "gpt_4o" {
  name          = "gpt-4o"
  provider_name = "openai"
  model         = "gpt-4o-2024-11-20"
  display_name  = "GPT-4o"
  visibility    = "public"
  api_key       = "$OPENAI_API_KEY"

  defaults_json = jsonencode({
    temperature = 0.2
  })

  request_headers_json = jsonencode({
    set = {
      "x-tenant" = "platform"
    }
  })
}

# An internal model: clients cannot request it directly, only a virtual model
# can route to it. provider_reference points at an agentgateway_llm_provider
# entry by name, and id pins the stable identity independently of the
# client-facing match pattern.
resource "agentgateway_llm_model" "claude_sonnet" {
  name               = "claude-*"
  id                 = "claude-sonnet"
  provider_reference = "shared-anthropic"
  model              = "claude-sonnet-4-5"
  visibility         = "internal"

  guardrails_json = jsonencode({
    streaming = "Enabled"
  })

  prompt_caching_json = jsonencode({
    cacheSystem   = true
    cacheMessages = true
  })
}
