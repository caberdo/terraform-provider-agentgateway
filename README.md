# terraform-provider-agentgateway

Terraform provider for the [agentgateway](https://agentgateway.dev/) standalone
control plane.

It manages LLM providers, models, virtual models, virtual API keys, MCP
targets, traffic gateways, routes, TCP routes and policies by translating
Terraform resources into writes on the agentgateway config resource API
(`/api/config/resources`). Every read is a `GET` on the running gateway; the
provider owns no truth of its own.

## Status

**v0.1.0.** The full v1 surface is implemented: 15 resources, 9 data sources,
import for every resource, and a generated reference in [`docs/`](docs/).

## Requirements

- Terraform 1.9 or later (plugin protocol 6.0).
- agentgateway v1.7.0 or later — the first release with the config resource
  API. The supported window lives in
  [`.github/agentgateway-versions.txt`](.github/agentgateway-versions.txt); the
  provider warns when it talks to an older gateway and never fails on a failed
  version probe.
- Gateway storage in `hybrid` mode for the full surface (see
  [Storage modes](#storage-modes)).
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

provider "agentgateway" {
  endpoint = "http://127.0.0.1:15000"
}
```

```sh
terraform init
terraform plan
```

To run an unreleased build, `make install` copies the binary into the local
plugin directory for the version reported by `git describe`, or point a
`dev_overrides` block in `~/.terraformrc` at `bin/`.

## Provider configuration

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

## Resources (15)

| Resource | Kind | Notes |
|---|---|---|
| `agentgateway_llm_provider` | `llm.provider` | Built-in or custom provider; credentials as `$ENV` references. |
| `agentgateway_llm_model` | `llm.model` | Public model bound to a provider by name. |
| `agentgateway_llm_virtual_model` | `llm.virtualModel` | Client-facing name routed to models by weight. |
| `agentgateway_llm_api_key` | `llm.apiKey` | Virtual key with metadata, `allowedModels` and budgets. Needs the `apiKey` policy. |
| `agentgateway_llm_policy` | `llm.policy` | By policy key: `apiKey`, `jwtAuth`, `basicAuth`, … |
| `agentgateway_llm_settings` | `llm.settings` | Singleton: gateways or the deprecated port/TLS listener. |
| `agentgateway_mcp_target` | `mcp.target` | `mcp`, `sse`, `stdio` and `openapi` transports. |
| `agentgateway_mcp_policy` | `mcp.policy` | By policy key: `authorization`, `jwtAuth`, … |
| `agentgateway_mcp_settings` | `mcp.settings` | Singleton: gateways/port, multiplexing and server info. |
| `agentgateway_traffic_gateway` | `traffic.gateway` | Listener ports, TLS and protocol options. |
| `agentgateway_traffic_route` | `traffic.route` | HTTP matches and backends, including backend policies. |
| `agentgateway_traffic_tcp_route` | `traffic.tcpRoute` | TCP backends (`host`, `backend`, `service`, `dynamic`). |
| `agentgateway_ui_policy` | `ui.policy` | By policy key: `cors`, `localRateLimit`, … |
| `agentgateway_frontend_policy` | `frontend.policy` | By policy key: `tls`, `accessLog`, … |
| `agentgateway_model_catalog` | `modelCatalog` | The DB-stored overlay of the model catalog. |

Every resource supports `terraform import`, including the
`<kind>/<id>`-qualified form, and documents its read-back contract in the
generated reference under [`docs/resources/`](docs/resources/).

## Data sources (9)

| Data source | Reads |
|---|---|
| `agentgateway_runtime` | Build and reload information from `/api/runtime`. |
| `agentgateway_llm_models` | Models, optionally prefix-filtered. |
| `agentgateway_llm_providers` | Providers, optionally prefix-filtered. |
| `agentgateway_llm_api_keys` | API-key metadata; never the key material. |
| `agentgateway_mcp_targets` | MCP targets. |
| `agentgateway_traffic_gateways` | Traffic gateways. |
| `agentgateway_traffic_routes` | Traffic routes, with their matches and backends. |
| `agentgateway_effective_config` | The effective config from `/api/config/effective`. Sensitive. |
| `agentgateway_config_resources` | Generic per-kind listing; the escape hatch for any kind or field. |

## Storage modes

`config.storage.mode` decides what the provider can manage. It is set in the
gateway's `config.yaml` and needs a restart to change.

| Mode | Support |
|---|---|
| `hybrid` | **Recommended.** The full surface: CRUD for every kind, stable revisions and generation checks. |
| `file` | Supported with limitations. Name-keyed kinds work, but every write rewrites `config.yaml`. The three singletons (`llm.settings`, `mcp.settings`, `modelCatalog`) are rejected because the list response omits them, and API keys are readable only when Terraform created them. |
| `readOnly` | Every write returns `403`. With the default `storage_mode_check = error` the provider fails during `Configure`, before any plan or apply touches the gateway. |

Settings fields the config file owns are owned by the file, not by Terraform:
the gateway answers `409 file-owned … cannot be updated` and the provider turns
that into an actionable diagnostic naming the field. It never works around it.

## Limitations

The v1 surface deliberately stops at these boundaries (spec §2, non-goals
N1–N9):

- **N1 — top-level `config:`.** `adminAddr`, `storage`, `database`, `session`,
  `dns`, `logging`, `tracing`, `metrics` and the model-catalog baseline are
  startup-only and live in `config.yaml` or Helm values. Only the DB-stored
  `modelCatalog` overlay is manageable.
- **N2 — `session.key`.** An internal kind; the API rejects it.
- **N3 — whole-file management.** `POST /api/config` and `/api/config/raw` can
  overwrite the baseline and bypass the resource model, so the provider does
  not expose them.
- **N4 — legacy `binds`.** Deprecated upstream in favour of `gateways` plus
  `routes`.
- **N5 — the Kubernetes CRD surface.** `AgentgatewayBackend`,
  `AgentgatewayPolicy`, `AgentgatewayParameters`, `AgentgatewayModel` and
  Gateway API are a different control plane with a different client, and out of
  scope for this provider.
- **N6 — runtime data.** Analytics, logs, traces, cost-catalog refresh and
  budget *status* are not configuration. Budget *definitions* are covered by
  `llm.apiKey.budgets`.
- **N7 — typed policy resources.** v1 ships four generic policy resources with
  `name` plus a JSON `spec`. Typed convenience resources are additive later.
- **N8 — data-plane verification.** The provider manages configuration, not
  traffic; the acceptance suite asserts stored state, not proxied requests.
- **N9 — settings fields owned by the config file.** Reported as a `409`
  diagnostic, never overridden.

Beyond the non-goals:

- A future gateway release that renames a kind or field is a provider release,
  not a configuration change; the `*_json` escape hatches keep unknown fields
  reachable in the meantime.
- Policy keys are a closed enum generated from the upstream schema; a new
  upstream policy needs a provider release.
- A `file`-mode gateway may assign positional ids to API keys, so keys created
  outside Terraform cannot always be imported.

## Verified unsupported

Each item below was checked against agentgateway v1.7.0 and is intentionally
not exposed. The source column points at the authoritative reason.

| Item | Why it is not in the provider | Source |
|---|---|---|
| `config.adminAddr`, `config.storage`, `config.database`, `config.session`, `config.dns`, `config.logging`, `config.tracing`, `config.metrics` | Startup-only baseline; the config resource API cannot express it | spec §2 N1; agentgateway config reference |
| `session.key` | Internal kind; the API answers `unsupported config resource kind` | spec §2 N2 |
| `POST /api/config`, `POST /api/config/raw` | Whole-file writes bypass the resource model | spec §2 N3 |
| Legacy `binds` | Deprecated upstream | spec §2 N4 |
| Kubernetes CRDs and Gateway API resources | Different control plane and client | spec §2 N5 |
| Analytics, logs, traces, cost-catalog refresh, budget status | Runtime data, not configuration | spec §2 N6 |
| Typed per-policy resources (`agentgateway_llm_jwt_auth_policy`, …) | Four generic policy resources cover every policy key | spec §2 N7, §15 D1 |
| Data-plane smoke traffic (real LLM/MCP requests) | The provider manages configuration only | spec §2 N8 |
| Settings fields owned by `config.yaml` | The gateway owns them; reported as a `409` diagnostic | spec §2 N9 |
| A 16th kind as a typed resource | `agentgateway_config_resources` already reaches any kind | spec §15 |

## Documentation

- [Architecture](ARCHITECTURE.md) — request flow, package layout, error taxonomy.
- [Contributing](CONTRIBUTING.md) — commands, review gates, contract-change protocol.
- [Changelog](CHANGELOG.md).
- [Releasing](contributing/releasing.md) — the maintainer release runbook.
- Generated reference: [`docs/`](docs/) (`make generate`).

## Coverage

<!-- coverage:start -->
![Coverage floor](https://img.shields.io/badge/coverage%20floor-80%25-green)

The coverage gate fails below **80%** (`make coverage-gate`).
<!-- coverage:end -->

## License

Mozilla Public License 2.0 — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).
