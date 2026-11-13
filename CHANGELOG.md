# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

## [0.1.0] - 2026-11-13

### Added

- Repository scaffold: Go module (`github.com/caberdo/terraform-provider-agentgateway`,
  Go 1.27.4), provider entry point (`providerserver.Serve` for
  `registry.terraform.io/caberdo/agentgateway`) and a provider stub.
- Tooling: `GNUmakefile` with the `build`, `test`, `coverage`, `lint`, `fmt`,
  `vet`, `validate-examples`, `generate`, `testacc-docker`, `testacc-matrix`,
  `coverage-gate`, `coverage-readme-check` and `release-check` targets;
  `.golangci.yml`; `.editorconfig`; `tfplugindocs` templates; and the coverage
  scripts (`scripts/coverage-gate.sh`, `scripts/coverage-summary.sh`,
  `scripts/merge-coverage.sh`, `scripts/coverage-readme.sh`).
- CI: lint/security, build/test, cross-platform build, generated-doc drift and
  coverage-gate workflows; a separate acceptance matrix workflow; the release
  tag and GoReleaser release pipeline; branch-protection drift, OpenSSF
  Scorecard and version-freshness workflows.
- Release: `terraform-registry-manifest.json` (plugin protocol 6.0), a
  GoReleaser configuration with signed checksums and provenance attestation,
  and the maintainer release runbook in `contributing/releasing.md`.
- Documentation set: `AGENTS.md`, `ARCHITECTURE.md`, `CONTRIBUTING.md` and
  this changelog.
- License: Apache License 2.0 (`LICENSE`).

### Known limitations

- The provider registers no resources and no data sources yet; the resource
  and data-source reference is generated into `docs/` once they land.
- The acceptance environment (`docker-compose.yml`) and the coverage floors
  (`.github/coverage-baseline.txt`, `.github/coverage-floors.txt`) are added
  with the acceptance and coverage tasks, so `make testacc-docker` and
  `make coverage-gate` cannot pass yet.
