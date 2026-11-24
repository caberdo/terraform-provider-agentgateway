# Terraform Provider for agentgateway

[![CI](https://img.shields.io/github/actions/workflow/status/caberdo/terraform-provider-agentgateway/test.yml?branch=main&label=CI)](https://github.com/caberdo/terraform-provider-agentgateway/actions/workflows/test.yml)
<!-- coverage-badge:start -->
[![Coverage floor](https://img.shields.io/badge/coverage%20floor-80%25-green)](#testing-and-coverage)
<!-- coverage-badge:end -->
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/caberdo/terraform-provider-agentgateway/badge)](https://scorecard.dev/viewer/?uri=github.com/caberdo/terraform-provider-agentgateway)
[![Release](https://img.shields.io/github/v/release/caberdo/terraform-provider-agentgateway?label=release)](https://github.com/caberdo/terraform-provider-agentgateway/releases)
[![Terraform Registry](https://img.shields.io/badge/registry-caberdo%2Fagentgateway-7b42bc)](https://registry.terraform.io/providers/caberdo/agentgateway)
[![OpenTofu](https://img.shields.io/badge/OpenTofu-available-ffda18)](https://search.opentofu.org/provider/caberdo/agentgateway)
[![Go](https://img.shields.io/github/go-mod/go-version/caberdo/terraform-provider-agentgateway)](https://golang.org/doc/devel/release.html)
[![License](https://img.shields.io/github/license/caberdo/terraform-provider-agentgateway)](LICENSE)
[![Last commit](https://img.shields.io/github/last-commit/caberdo/terraform-provider-agentgateway)](https://github.com/caberdo/terraform-provider-agentgateway/commits/main)

Manage the **[agentgateway](https://agentgateway.dev/)** standalone control plane as code. The
provider translates Terraform resources into writes on the gateway's config resource API
(`/api/config/resources`) and manages LLM providers, models, virtual models, virtual API keys, MCP
targets, traffic gateways, routes, TCP routes and policies. Every read is a `GET` on the running
gateway; the provider owns no truth of its own.

**15 resources and 9 data sources.**

| | |
|---|---|
| 🎯 **The whole config surface** | LLM providers, models, virtual models and API keys, MCP targets and policies, traffic gateways, HTTP and TCP routes, UI and frontend policies, and the model-catalog overlay |
| 🔁 **Read back, not fire-and-forget** | Every attribute is re-read from the running gateway, so state describes what the gateway stored rather than what Terraform sent |
| 📦 **Import everywhere** | Every resource supports `terraform import`, including the `<kind>/<id>`-qualified form |
| 🧪 **Tested against a real gateway** | The acceptance suite runs against agentgateway v1.7.0 in Docker, across the four gateway services that carry the negative paths (`hybrid`, `readonly`, `fileowned`, `filemode`), plus the version window in `.github/agentgateway-versions.txt` |
| 🧭 **Honest boundaries** | `readOnly` storage fails at `Configure`, a config-file-owned field is diagnosed by name as `409 file-owned`, and every verified gap is listed under [Verified unsupported](#verified-unsupported) |

## Contents

- [Quick start](#quick-start)
- [Requirements and compatibility](#requirements-and-compatibility)
- [Storage modes](#storage-modes)
- [Limitations](#limitations)
- [Verified unsupported](#verified-unsupported)
- [Provider configuration](#provider-configuration)
- [Resources](#resources)
- [Data sources](#data-sources)
- [Examples](#examples)
- [Verifying a release](#verifying-a-release)
- [Testing and coverage](#testing-and-coverage)
- [Development](#development)
- [Troubleshooting and FAQ](#troubleshooting-and-faq)
- [Security, contributing and support](#security-contributing-and-support)
- [License](#license)

## Quick start

```terraform
terraform {
  required_providers {
    agentgateway = {
      source  = "caberdo/agentgateway"
      version = "~> 0.2"
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

To run an unreleased build, `make install` copies the binary into the local plugin directory for
the version reported by `git describe`, or point a `dev_overrides` block in `~/.terraformrc` at
`bin/`.

[`examples/complete`](./examples/complete) is a working module that manages one of each integration
surface against a local gateway, in dependency order.

The current release is **v0.1.0**: the full v1 surface is implemented, every resource supports
`terraform import`, and the generated reference lives in [`docs/`](docs/).

## Requirements and compatibility

| | |
|---|---|
| Terraform | 1.9 or later (plugin protocol 6.0) |
| agentgateway | v1.7.0 or later — the first release with all 15 config resource kinds (`llm.settings` arrived in v1.6.0, `frontend.policy` in v1.7.0). The supported window lives in [`.github/agentgateway-versions.txt`](./.github/agentgateway-versions.txt); the provider warns when it talks to an older gateway and never fails on a failed version probe |
| Gateway storage | `hybrid` mode for the full surface — see [Storage modes](#storage-modes) |
| Go (to build from source) | 1.27.4 |

## Storage modes

`config.storage.mode` decides what the provider can manage. It is set in the gateway's `config.yaml`
and needs a restart to change.

| Mode | Support |
|---|---|
| `hybrid` | **Recommended.** The full surface: CRUD for every kind, stable revisions and generation checks. |
| `file` | Supported with limitations. Name-keyed kinds work, but every write rewrites `config.yaml`. The three singletons (`llm.settings`, `mcp.settings`, `modelCatalog`) are rejected because the list response omits them, and API keys are readable only when Terraform created them. |
| `readOnly` | Every write returns `403`. With the default `storage_mode_check = error` the provider fails during `Configure`, before any plan or apply touches the gateway. |

Settings fields the config file owns are owned by the file, not by Terraform: the gateway answers
`409 file-owned … cannot be updated` and the provider turns that into an actionable diagnostic
naming the field. It never works around it.

## Limitations

The v1 surface deliberately stops at these boundaries (spec §2, non-goals N1–N9):

- **N1 — top-level `config:`.** `adminAddr`, `storage`, `database`, `session`, `dns`, `logging`,
  `tracing`, `metrics` and the model-catalog baseline are startup-only and live in `config.yaml` or
  Helm values. Only the DB-stored `modelCatalog` overlay is manageable.
- **N2 — `session.key`.** An internal kind; the API rejects it.
- **N3 — whole-file management.** `POST /api/config` and `/api/config/raw` can overwrite the
  baseline and bypass the resource model, so the provider does not expose them.
- **N4 — legacy `binds`.** Deprecated upstream in favour of `gateways` plus `routes`.
- **N5 — the Kubernetes CRD surface.** `AgentgatewayBackend`, `AgentgatewayPolicy`,
  `AgentgatewayParameters`, `AgentgatewayModel` and Gateway API are a different control plane with a
  different client, and out of scope for this provider.
- **N6 — runtime data.** Analytics, logs, traces, cost-catalog refresh and budget *status* are not
  configuration. Budget *definitions* are covered by `llm.apiKey.budgets`.
- **N7 — typed policy resources.** v1 ships four generic policy resources with `name` plus a JSON
  `spec`. Typed convenience resources are additive later.
- **N8 — data-plane verification.** The provider manages configuration, not traffic; the acceptance
  suite asserts stored state, not proxied requests.
- **N9 — settings fields owned by the config file.** Reported as a `409` diagnostic, never
  overridden.

Beyond the non-goals:

- A future gateway release that renames a kind or field is a provider release, not a configuration
  change; the `*_json` escape hatches keep unknown fields reachable in the meantime.
- Policy keys are a closed enum generated from the upstream schema; a new upstream policy needs a
  provider release.
- A `file`-mode gateway may assign positional ids to API keys, so keys created outside Terraform
  cannot always be imported.

## Verified unsupported

Each item below was checked against agentgateway v1.7.0 and is intentionally not exposed. The source
column points at the authoritative reason.

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

The admin address has **no authentication** by default. Treat network isolation as the primary
control and use the token/basic/header options only on the gateway-served UI/API path.

## Resources

Every resource links to its generated reference under [`docs/resources/`](docs/resources/).

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

Every resource supports `terraform import`, including the `<kind>/<id>`-qualified form, and
documents its read-back contract in the generated reference under
[`docs/resources/`](docs/resources/).

## Data sources

The generated reference for every data source is under [`docs/data-sources/`](docs/data-sources/).

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

## Examples

| Path | What it shows |
|---|---|
| [`examples/complete`](./examples/complete) | An end-to-end module managing one of each integration surface in dependency order |
| [`examples/resources/<resource>`](./examples/resources) | One `resource.tf` per resource, used to generate the docs |
| [`examples/data-sources/<data source>`](./examples/data-sources) | One `data-source.tf` per data source |

`make validate-examples` checks every example against the provider schema.

## Verifying a release

Every release is built by [GoReleaser](https://github.com/goreleaser/goreleaser) from a GPG-signed,
GitHub-verified tag, and ships eleven assets:

| Asset | Purpose |
|---|---|
| `terraform-provider-agentgateway_<version>_{darwin,linux,windows}_{amd64,arm64}.zip` | The six provider binaries |
| `terraform-provider-agentgateway_<version>_SHA256SUMS` | Checksums for every artifact |
| `terraform-provider-agentgateway_<version>_SHA256SUMS.sig` | GPG signature over the checksums |
| `terraform-provider-agentgateway_<version>_manifest.json` | Terraform registry manifest (plugin protocol versions) |
| `terraform-provider-agentgateway_<version>_provenance.intoto.jsonl` | Build provenance over every checksummed artifact |
| `sbom.spdx.json` | SPDX software bill of materials |

Replace `<version>` with the release you are verifying. Start with the release itself:

```bash
gh release view v<version> --repo caberdo/terraform-provider-agentgateway \
  --json tagName,isDraft,isPrerelease,assets \
  --jq '{tagName, isDraft, isPrerelease, assets: [.assets[].name]}'
```

A healthy release is neither draft nor prerelease and carries all eleven assets. Download and verify
the checksums:

```bash
WORK=$(mktemp -d)
cd "$WORK"
gh release download v<version> --repo caberdo/terraform-provider-agentgateway \
  --pattern 'terraform-provider-agentgateway_<version>_*'
gpg --verify terraform-provider-agentgateway_<version>_SHA256SUMS.sig \
  terraform-provider-agentgateway_<version>_SHA256SUMS
shasum -a 256 -c terraform-provider-agentgateway_<version>_SHA256SUMS   # macOS
# sha256sum -c terraform-provider-agentgateway_<version>_SHA256SUMS      # Linux
```

`shasum`/`sha256sum` must report every archive and the manifest as `OK`. If `gpg` reports `Can't
check signature: No public key`, import the signing key first:

```bash
gh api users/dennismdejong/gpg_keys --jq '.[].raw_key' | gpg --import
```

The checksum signature proves the checksums are ours; the build provenance proves *where the
binaries come from*:

```bash
gh attestation verify terraform-provider-agentgateway_<version>_linux_amd64.zip \
  --repo caberdo/terraform-provider-agentgateway
```

That exits 0 only when the archive was built by the release workflow. Verify the signed tag through
the API as well — the tag must resolve to a tag object with a valid signature:

```bash
TAG_OBJECT=$(gh api repos/caberdo/terraform-provider-agentgateway/git/ref/tags/v<version> --jq .object.sha)
gh api "repos/caberdo/terraform-provider-agentgateway/git/tags/$TAG_OBJECT" \
  --jq '.verification | {verified, reason}'
```

Expect `{"verified":true,"reason":"valid"}`. The full runbook, including the Registry, manifest and
SBOM checks and the recovery path for a failed publish, is in
[`contributing/releasing.md`](./contributing/releasing.md#4-verify).

## Testing and coverage

| Command | What it runs |
|---|---|
| `make test` | Unit tests (`TestAcc*` skips itself without `TF_ACC`) |
| `make coverage` | Unit tests with a statement-coverage profile and per-function listing |
| `make coverage-gate` | Fails when the profile is below the floor in [`.github/coverage-baseline.txt`](./.github/coverage-baseline.txt) or a package floor in [`.github/coverage-floors.txt`](./.github/coverage-floors.txt) |
| `make testacc-docker` | The acceptance suite against agentgateway in Docker |
| `make testacc-matrix` | The acceptance suite against every supported agentgateway version |
| `make coverage-merge` | Merges the unit profile with the acceptance profile into one number |

The acceptance suite needs four gateway services because one instance cannot produce every negative
path: `403` needs the read-only service, `409 file-owned` needs a gateway whose config file owns the
field, and the file-mode singleton rejection needs a gateway in `config.storage.mode: file`.
`AGENTGATEWAY_EXPECT_VERSION` is set by the matrix and by CI, so a stale image cannot masquerade as a
green run.

<!-- coverage:start -->
The coverage gate fails below **80%** (`make coverage-gate`).
<!-- coverage:end -->

## Development

```bash
make build              # build bin/terraform-provider-agentgateway
make install            # install into ~/.terraform.d/plugins for local dev_overrides
make test               # unit tests
make coverage           # unit coverage profile and summary
make lint               # golangci-lint (.golangci.yml) and the gofmt gate
make vet                # go vet ./...
make fmt                # go fmt ./...
make generate           # regenerate docs/ with tfplugindocs
make validate-examples  # check examples/ against the provider schema
```

To use a local build with Terraform, add to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "caberdo/agentgateway" = "/path/to/terraform-provider-agentgateway/bin"
  }
}
```

`main` is protected: every change lands through a pull request with green CI. [`CONTRIBUTING.md`](./CONTRIBUTING.md)
has the commands, review gates and contract-change protocol, and [`AGENTS.md`](./AGENTS.md) holds
the repository conventions.

## Troubleshooting and FAQ

**The provider fails during `Configure` against a read-only gateway.**
Every write on `readOnly` storage returns `403`, and with the default `storage_mode_check = error`
the provider refuses to start. Plan against a gateway in `hybrid` (or `file`) storage, or lower
`storage_mode_check` to `warn` or `ignore` when you only need to read.

**A settings field fails with `409 file-owned … cannot be updated`.**
The gateway's `config.yaml` owns that field. Change it in the file and restart the gateway; the
provider will not overwrite it.

**An API key created outside Terraform cannot be imported.**
See [the `file`-mode id caveat](#limitations). Keys Terraform created are always readable.

**The gateway is older than v1.7.0.**
The provider warns and continues; it never fails on a failed version probe. Upgrade the gateway to
a release in [`.github/agentgateway-versions.txt`](./.github/agentgateway-versions.txt) for the full
surface.

**`Configure` fails while probing `/api/runtime`.**
Set `verify = false` to skip the probe.

**Does the admin address need credentials?**
No — it has no authentication by default. Treat network isolation as the primary control.

## Security, contributing and support

- [`CONTRIBUTING.md`](./CONTRIBUTING.md) — commands, review gates and the contract-change protocol.
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — request flow, package layout and error taxonomy.
- [`CHANGELOG.md`](./CHANGELOG.md) — what changed in every release.
- [`contributing/releasing.md`](./contributing/releasing.md) — the maintainer release runbook.

## License

MIT — see the [LICENSE](LICENSE) file, and [`NOTICE`](./NOTICE) for the upstream
works this project derives from and the licence carve-outs that apply to them.
