# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

## [0.1.0] - 2026-11-13

The first release: the complete v1 surface, 15 resources and 9 data sources,
against agentgateway v1.7.0 and later.

### Added

- **Provider** `agentgateway` (plugin protocol 6.0) with no auth, bearer token,
  basic auth, static headers and full TLS/mTLS options; environment-variable
  fallbacks, cross-field validation, a `/api/runtime` version probe and the
  `storage_mode_check` policy for `readOnly` storage.
- **Resources (15):** `agentgateway_llm_provider`, `agentgateway_llm_model`,
  `agentgateway_llm_virtual_model`, `agentgateway_llm_api_key`,
  `agentgateway_llm_policy`, `agentgateway_llm_settings`,
  `agentgateway_mcp_target`, `agentgateway_mcp_policy`,
  `agentgateway_mcp_settings`, `agentgateway_traffic_gateway`,
  `agentgateway_traffic_route`, `agentgateway_traffic_tcp_route`,
  `agentgateway_ui_policy`, `agentgateway_frontend_policy` and
  `agentgateway_model_catalog`. Every resource supports `terraform import`,
  including the `<kind>/<id>`-qualified form, and preserves attributes the
  server did not return instead of zeroing them.
- **Data sources (9):** `agentgateway_runtime`, `agentgateway_llm_models`,
  `agentgateway_llm_providers`, `agentgateway_llm_api_keys`,
  `agentgateway_mcp_targets`, `agentgateway_traffic_gateways`,
  `agentgateway_traffic_routes`, `agentgateway_effective_config` (sensitive)
  and the generic `agentgateway_config_resources`.
- **Error handling:** typed sentinels for not-found, invalid, conflict,
  read-only, transient and server errors; retries with backoff for transient
  failures; actionable diagnostics for `403` read-only storage, `409`
  file-owned settings fields, the missing `llm.policies.apiKey` prerequisite,
  and ids that exist outside Terraform state.
- **Backend policies:** `agentgateway_traffic_route` and
  `agentgateway_traffic_tcp_route` expose per-backend `policies_json` so
  out-of-band backend TLS and authorization policies are not dropped on the
  next apply.
- **Documentation:** generated resource and data-source reference in `docs/`,
  a complete example in `examples/complete`, and the architecture,
  contributing and agent guides.
- **Tooling and CI:** `GNUmakefile` (`build`, `test`, `coverage`, `lint`,
  `fmt-check`, `vet`, `validate-examples`, `generate`, `testacc-docker`,
  `testacc-matrix`, `coverage-gate`, `coverage-readme-check`,
  `release-check`), a gofmt gate, golangci-lint, gosec, govulncheck, generated
  doc drift detection, per-package coverage floors and the acceptance matrix
  across the supported agentgateway window.
- **Release:** `terraform-registry-manifest.json`, a GoReleaser configuration
  with signed checksums and provenance attestation, and the maintainer release
  runbook in `contributing/releasing.md`.

### Storage modes

- `hybrid` is the supported mode and the only one with the full surface.
- `file` works for name-keyed kinds with documented limitations: the three
  singletons (`llm.settings`, `mcp.settings`, `modelCatalog`) are rejected and
  API keys are readable only when Terraform created them.
- `readOnly` fails fast during `Configure` unless `storage_mode_check` is
  lowered to `warn` or `ignore`.

### Known limitations

- The non-goals in spec §2 N1–N9 are not managed: the top-level `config:`
  baseline, `session.key`, whole-file writes, legacy `binds`, the Kubernetes
  CRD surface, runtime analytics/logs/traces, typed per-policy resources,
  data-plane verification and settings fields owned by the config file.
- Policy keys are a closed enum generated from the upstream schema; a new
  upstream policy needs a provider release.
- The provider manages configuration, not traffic. Acceptance tests assert
  stored state, not proxied requests.
