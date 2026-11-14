# Complete end-to-end configuration for a local agentgateway.
#
# Everything here is managed through the provider: an upstream LLM provider and
# the model that uses it, a virtual model in front of that model, the API-key
# policy and a virtual key, an MCP target, and a traffic gateway with one route.
# The data source reads the gateway build version back.
#
# Apply it against a local gateway started with `agentgateway -f config.yaml`
# and its admin API on http://127.0.0.1:15000.

terraform {
  required_version = ">= 1.9"

  required_providers {
    agentgateway = {
      source = "caberdo/agentgateway"
    }
  }
}

provider "agentgateway" {
  endpoint = "http://127.0.0.1:15000"
}

# 1. Upstream provider. `openai` is a built-in provider name; the credential is
#    an environment reference so the key never lands in Terraform state.
resource "agentgateway_llm_provider" "openai" {
  name          = "openai-local"
  provider_name = "openai"
  model         = "gpt-4o-mini"
  api_key       = "$OPENAI_API_KEY"
  base_url      = "https://api.openai.com"
}

# 2. The public model clients request. It inherits the provider entry above by
#    name, so credentials and upstream defaults stay in one place.
resource "agentgateway_llm_model" "chat" {
  name               = "gpt-4o-mini"
  provider_reference = agentgateway_llm_provider.openai.name
  model              = "gpt-4o-mini"
  display_name       = "GPT-4o mini"
  visibility         = "public"
}

# 3. A stable client-facing name in front of the concrete model. A single
#    weighted target keeps the routing block exercised without splitting
#    traffic; add more targets with weights to canary a second model.
resource "agentgateway_llm_virtual_model" "chat" {
  name = "chat"

  weighted {
    targets {
      model  = agentgateway_llm_model.chat.name
      weight = 100
    }
  }
}

# 4. The apiKey policy enables virtual-key authentication on the LLM listener.
#    Keys live in agentgateway_llm_api_key resources, so the spec must not
#    contain a `keys` list.
resource "agentgateway_llm_policy" "api_key" {
  name = "apiKey"

  spec = jsonencode({
    mode = "strict"
  })
}

# 5. A virtual key for local development. It depends on the policy above
#    because the gateway rejects keys without it.
resource "agentgateway_llm_api_key" "developer" {
  key = "local-development-key"

  metadata = {
    name  = "Local Developer"
    user  = "developer"
    group = "platform"
  }

  allowed_models = [agentgateway_llm_virtual_model.chat.name]

  depends_on = [agentgateway_llm_policy.api_key]
}

# 6. An MCP target on a local MCP server. The full-URI form of `host` carries
#    the scheme and port; `path` selects the streamable HTTP endpoint.
resource "agentgateway_mcp_target" "local" {
  name = "local-tools"

  mcp = {
    host = "http://127.0.0.1:8080"
    path = "/mcp"
  }
}

# 7. The traffic listener. Routes attach to it by name.
resource "agentgateway_traffic_gateway" "local" {
  name         = "local"
  port         = 4000
  bind_address = "127.0.0.1"
}

# 8. An HTTP route on that gateway: every request under /api is forwarded to a
#    service on the host.
resource "agentgateway_traffic_route" "api" {
  name     = "local-api"
  gateways = [agentgateway_traffic_gateway.local.name]

  matches {
    path {
      prefix = "/api"
    }
  }

  backends {
    host = "127.0.0.1:8080"
  }
}

# 9. Build and reload information reported by the gateway's admin API.
data "agentgateway_runtime" "current" {}

output "build_version" {
  description = "Build version reported by the local gateway."
  value       = data.agentgateway_runtime.current.build_version
}
