# Complete example

An end-to-end module for a local agentgateway. It manages one of each
integration surface in dependency order:

| Resource | Purpose |
|---|---|
| `agentgateway_llm_provider.openai` | Built-in `openai` provider with an `$OPENAI_API_KEY` reference |
| `agentgateway_llm_model.chat` | Public model that inherits the provider by name |
| `agentgateway_llm_virtual_model.chat` | Client-facing `chat` name routed to that model by weight |
| `agentgateway_llm_policy.api_key` | The `apiKey` policy, without inline keys |
| `agentgateway_llm_api_key.developer` | Virtual key with `name`/`user`/`group` metadata |
| `agentgateway_mcp_target.local` | Streamable HTTP MCP target on a local MCP server |
| `agentgateway_traffic_gateway.local` | HTTP listener on port 4000 |
| `agentgateway_traffic_route.api` | `/api` prefix route with a host backend |
| `data.agentgateway_runtime.current` | Build and reload information |

## Usage

```sh
terraform init
terraform apply
```

The provider talks to the admin API on `http://127.0.0.1:15000`. Start a local
gateway first, and export `OPENAI_API_KEY` for the upstream provider. Replace
the placeholder key and upstream values before using this against a real
deployment.

The example is validated without a running gateway:

```sh
make validate-examples
```

`terraform validate` only checks the configuration, so the `agentgateway_runtime`
data source is not read until `terraform plan` or `terraform apply`.
