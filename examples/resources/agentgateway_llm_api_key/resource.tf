# Virtual keys authenticate LLM requests. Exactly one of `key` and `key_hash`
# is set; changing it is an in-place rotation and the original key stops
# authenticating.
#
# Every key needs the `apiKey` LLM policy. Manage it in Terraform:
resource "agentgateway_llm_policy" "api_key" {
  name = "apiKey"

  spec = jsonencode({
    mode = "strict"
  })
}

# ... or add `llm.policies.apiKey` to the gateway configuration file. Without
# one, the apply fails with a diagnostic that names this prerequisite.

variable "team_a_key" {
  description = "Plaintext virtual key for team A."
  type        = string
  sensitive   = true
}

resource "agentgateway_llm_api_key" "team_a" {
  key      = var.team_a_key
  metadata = { team = "team-a", owner = "platform" }

  # Omit `allowed_models` for no constraint; an empty list denies every model.
  allowed_models = ["gpt-4o-mini", "openai/*"]

  # Budgets are charged independently after each LLM response. `Tokens` caps
  # usage, `USD` caps realized cost and needs a priced catalog.
  budgets = [
    {
      name               = "daily-spend"
      limit              = { unit = "USD", amount = 50 }
      window             = { rolling = "24h" }
      on_budget_exceeded = "Block"
    },
    {
      # No window: the budget accumulates for its lifetime and never resets.
      name               = "lifetime-tokens"
      limit              = { unit = "Tokens", amount = 250000 }
      on_budget_exceeded = "Audit"
    },
  ]
}

# The hashed variant keeps the plaintext key out of Terraform state. The hash is
# `sha256:<64 lowercase hex characters>`, for example the output of
# `printf %s "$KEY" | sha256sum`.
resource "agentgateway_llm_api_key" "ci" {
  key_hash = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
  metadata = { team = "ci" }

  # Deny every model: this key may authenticate but not request anything yet.
  allowed_models = []
}
