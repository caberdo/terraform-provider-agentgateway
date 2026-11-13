# AGENTS.md

Guidance for coding agents working in this repository.

## Read these before changing behaviour

The binding design and the implementation plan are **not committed**. The
maintainer keeps them in the main checkout under `docs/superpowers/`:

1. `docs/superpowers/specs/2026-11-13-terraform-provider-agentgateway-design.md`
   — the binding design: constraints (C1–C25), frozen interfaces, resource
   contracts and the read-back matrix.
2. `docs/superpowers/plans/2026-11-13-terraform-provider-agentgateway-v1.md` —
   the task-by-task implementation plan and lane ownership.

Those paths are gitignored on purpose, so **a linked worktree does not contain
them**. Work that runs under SDD is given what it needs instead:

- the brief for its own task at `.superpowers/sdd/<plan>/task-<N>-brief.md`,
- the shared templates beside it in the same directory
  (`resource-task-template.md`, `datasource-task-template.md`), and
- `progress.md`, which records the state of the plan.

Two further documents *are* committed, and are therefore readable everywhere:

3. `ARCHITECTURE.md` — how the pieces fit together.
4. `CONTRIBUTING.md` — commands, review gates and the contract-change protocol.

The spec is the authority. If code and spec disagree, the spec wins or the spec
is changed deliberately, in the same change.

## Documentation map

| File | Role | Edit by hand? |
|---|---|---|
| `docs/superpowers/specs/2026-11-13-terraform-provider-agentgateway-design.md` | Binding design: constraints, frozen interfaces, resource contracts, read-back matrix. | Yes, deliberately, in the same change as the code it governs. |
| `docs/superpowers/plans/2026-11-13-terraform-provider-agentgateway-v1.md` | Task-by-task plan, lane ownership, verification commands. | Yes, when scope changes. |
| `README.md` | User-facing install, configuration, resource tables, limitations. | Yes. |
| `ARCHITECTURE.md` | Request flow, client internals, error taxonomy, auth matrix, test layers. | Yes. |
| `CONTRIBUTING.md` | Workflow, review gates, branch protection and merge protocol. | Yes. |
| `docs/index.md`, `docs/resources/*.md`, `docs/data-sources/*.md` | Generated reference. | **No.** Run `make generate`. |
| `examples/**` | Validated modules; also the source of the generated examples. | Yes, then `make validate-examples` and `make generate`. |
| `templates/**` | `tfplugindocs` templates. | Yes, then `make generate`. |

## Package layout

```
main.go                     provider entry point (providerserver.Serve)
internal/provider/          provider schema, config validation, registration
internal/providerdata/      typed data handed to resources
internal/client/            HTTP client, endpoints, retries, error taxonomy
internal/configresource/    Kind, Resource, id derivation, managed metadata, codec
internal/resourceutil/      import IDs, plan modifiers, JSON/state helpers, CRUD harness
internal/resources/<name>/  one package per resource, tests co-located
internal/datasources/<name>/ one package per data source, tests co-located
internal/agentgatewaytest/  fake config-resource server, fixtures and harness
internal/acctest/           TF_ACC guards and acceptance helpers
```

Do not create a flat `internal/provider/resource_*.go` layout. The per-resource
packages are what make parallel work safe: no shared files, and tests travel
with the code they cover.

## Commands

| Command | Purpose |
|---|---|
| `make build` | build `bin/terraform-provider-agentgateway` |
| `make test` | unit tests |
| `make coverage` | unit coverage profile and summary |
| `make lint` | golangci-lint (config at `.golangci.yml`) |
| `make fmt` / `make vet` | formatting and vet |
| `make validate-examples` | `terraform fmt`/`validate` every example |
| `make generate` | regenerate `docs/` with `tfplugindocs` |
| `make testacc-docker` | acceptance suite against agentgateway in Docker |
| `make testacc-matrix` | acceptance suite against every supported agentgateway version |
| `make coverage-gate` | fail below the global floor in `.github/coverage-baseline.txt` or a package floor in `.github/coverage-floors.txt` |
| `make coverage-readme-check` | fail when the README coverage badge drifts |
| `make release-check` | fail when `VERSION`, `CHANGELOG.md` and the README disagree |

## Rules

- Only the lead edits `go.mod`, `go.sum`, `internal/provider/**`, CI files and
  generated docs. Lane work stays inside its owned paths.
- Always pin the latest stable Go release in `go.mod` (currently `go 1.27.4`);
  bump it as part of the change that needs it.
- License is MPL-2.0 (`LICENSE`); third-party derivations are recorded in
  `NOTICE`. Do not copy source from an Apache-2.0 or AGPL project without adding
  it there.
- No new third-party dependency without the lead's approval.
- No `panic`, `log.Fatal` or `os.Exit` outside `main`.
- No secret in a diagnostic, log field, test failure message or committed file.
  `tflog` logs only kind, id, status and duration.
- Every `*_json` attribute is `jsontypes.Normalized`; omitted `*_json`
  attributes are omitted from the payload.
- Attributes the server did not return are preserved from state, never zeroed.
- `Create` refuses an id that already exists outside state and points at
  `terraform import`; it never adopts silently.
- Policy `name` changes are `RequiresReplace`; every other rename is in place.
- Exactly 15 resources and 9 data sources, named as in spec §7/§8. No extra
  abstractions over kinds.
- Docs under `docs/` are generated. Never hand-edit them.
- Coverage must be checked before claiming work is done, and it must never drop
  below the global floor in `.github/coverage-baseline.txt` or a package floor
  in `.github/coverage-floors.txt`. Run `make coverage` (and the acceptance
  suite where relevant) and confirm `./scripts/coverage-gate.sh` passes before
  opening or merging a pull request; if a change adds uncovered code, add tests
  or lower the floor deliberately in the same change.

## Contract-change protocol

Shared interfaces are frozen in the spec §5. If a change needs a different
signature, attribute name, value shape or import format:

1. Stop. Do not improvise the change in a resource package.
2. Update the spec in the same change, with the reason.
3. Update every consumer (resources, data sources, provider, tests, docs) in
   that change.
4. Note the change in the pull request and the changelog if it is user-visible.

A contract change that lands without the spec is a review finding regardless of
how well it works.

## Review gates

A change is ready when `make build`, `make vet`, `make lint` and the relevant
`go test` scope pass, generated docs show no diff, and no write escaped the
task's owned paths. Acceptance tests must compile even when they are not run
locally.

## Lane history

| Wave | Work | Where it lives |
|---|---|---|
| 0 | Scaffold: repository tooling, CI, provider entry point and provider stub | `main` |
| 1 | Foundation: `configresource`, `client` + protocol fake, `providerdata` + provider config, `resourceutil` CRUD harness, acceptance environment | `internal/configresource`, `internal/client`, `internal/agentgatewaytest`, `internal/providerdata`, `internal/provider`, `internal/resourceutil`, `internal/acctest` |
| 2 | Ten resource/data-source lanes in parallel | `internal/resources/*`, `internal/datasources/*`, `examples/**` |
| 3 | Integration: wiring and generated docs, complete example, acceptance/coverage/release, whole-branch review | `internal/provider` registration, `docs/`, `examples/complete`, CI, `README.md` |

Lane ownership existed to keep parallel work conflict-free: each lane owned its
resource package, tests and example, and no lane wrote `internal/provider`. The
integration wave is serial on purpose because it edits the shared files
(registration, generated docs, CI) that were frozen during the lane wave.
