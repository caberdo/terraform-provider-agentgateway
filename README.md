# terraform-provider-agentgateway

Terraform provider for the [agentgateway](https://agentgateway.dev/) standalone
control plane.

It manages LLM providers, models, virtual models, virtual API keys, MCP
targets, traffic gateways, routes, TCP routes and policies by translating
Terraform resources into writes on the agentgateway config resource API
(`/api/config/resources`). Every read is a `GET` on the running gateway; the
provider owns no truth of its own.

## Status

Pre-release. The provider is under active development; the resource and data
source reference is generated into [`docs/`](docs/) once the resources land.

## Requirements

- Terraform 1.9 or later (plugin protocol 6.0).
- agentgateway v1.7.0 or later — the first release with the config resource
  API. The provider warns when it talks to an older gateway and never fails on
  a failed version probe.
- Go 1.27.4 to build from source.

## Installation

```terraform
terraform {
  required_providers {
    agentgateway = {
      source  = "caberdo/agentgateway"
      version = "~> 0.1"
    }
  }
}
```

## Provider configuration

```terraform
provider "agentgateway" {
  endpoint = "http://127.0.0.1:15000"
}
```

| Attribute | Environment variable | Default | Notes |
|---|---|---|---|
| `endpoint` | `AGENTGATEWAY_ENDPOINT` | `http://127.0.0.1:15000` | Admin address, or the gateway port that serves the UI/API. No `/ui` or `/api` suffix. |
| `token` | `AGENTGATEWAY_TOKEN` | – | Bearer token. Conflicts with `username`/`password`. |
| `username` | `AGENTGATEWAY_USERNAME` | – | Basic auth; requires `password`. |
| `password` | `AGENTGATEWAY_PASSWORD` | – | Requires `username` and an `https` endpoint. |
| `http_headers` | – | – | Static headers added to every request. |
| `timeout` | `AGENTGATEWAY_TIMEOUT` | `30s` | Per-request deadline. |
| `ca_cert` / `ca_cert_file` | `AGENTGATEWAY_CA_CERT` / `_FILE` | – | Mutually exclusive. |
| `client_cert` / `client_cert_file` | `AGENTGATEWAY_CLIENT_CERT` / `_FILE` | – | mTLS. |
| `client_key` / `client_key_file` | `AGENTGATEWAY_CLIENT_KEY` / `_FILE` | – | mTLS. |
| `insecure_skip_verify` | `AGENTGATEWAY_INSECURE_SKIP_VERIFY` | `false` | Testing only. |
| `verify` | `AGENTGATEWAY_VERIFY` | `true` | Probe `/api/runtime` during `Configure`. |
| `storage_mode_check` | – | `error` | `error` \| `warn` \| `ignore`. Concerns `readOnly` storage only. |

The admin address has **no authentication** by default. Treat network
isolation as the primary control and use the token/basic/header options only
on the gateway-served UI/API path.

## Documentation

- [Architecture](ARCHITECTURE.md) — request flow, package layout, error taxonomy.
- [Contributing](CONTRIBUTING.md) — commands, review gates, contract-change protocol.
- [Changelog](CHANGELOG.md).
- [Releasing](contributing/releasing.md) — the maintainer release runbook.

## License

Apache License 2.0 — see [`LICENSE`](LICENSE).
