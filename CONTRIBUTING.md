# Contributing

Thanks for contributing to the Terraform provider for agentgateway.

## Before you start

Read, in this order:

1. `docs/superpowers/specs/2026-11-13-terraform-provider-agentgateway-design.md`
   — kept locally (gitignored); obtain from the maintainer if missing.
2. `docs/superpowers/plans/2026-11-13-terraform-provider-agentgateway-v1.md` —
   kept locally (gitignored); obtain from the maintainer if missing.
3. [`ARCHITECTURE.md`](ARCHITECTURE.md) and [`AGENTS.md`](AGENTS.md)

The spec is the binding authority for interfaces and read-back behaviour. If a
change needs a different contract, say so in the pull request and update the
spec in the same change; do not improvise a shared interface.

## Development setup

Requirements:

- Go 1.27.4 (the version pinned in `go.mod`). `GOTOOLCHAIN=auto` downloads the
  pinned toolchain when the local Go is older.
- Terraform 1.9 or later for example validation.
- Docker Compose (or `podman-compose`) for acceptance tests.

```bash
make build
make test
make lint
make vet
make validate-examples
make generate
make coverage
make coverage-gate
```

## Acceptance tests and the version matrix

`.github/agentgateway-versions.txt` is the single source of truth: the declared
floor on the first line, then the rolling window, ascending, one per line.

- A pull request runs the floor and the newest release (`Acceptance Tests
  (agentgateway <version>)`); `main` and the merge queue run the whole window.
- Run one version locally with `make testacc-docker AGENTGATEWAY_VERSION=v1.7.0`,
  or a subset of the window with
  `AGENTGATEWAY_VERSIONS="v1.7.0 v1.6.0" make testacc-matrix`. The matrix gives
  each version its own compose project and prints a PASS/FAIL summary.
- The compose environment runs four gateway services: `hybrid` (15000),
  `readonly` (15001), `fileowned` (15002) and `filemode` (15003). One instance
  cannot produce every negative path — `403` needs the read-only service,
  `409 file-owned` needs a gateway whose config file owns the field, and the
  file-mode singleton rejection needs a gateway in `config.storage.mode: file`.
- `AGENTGATEWAY_EXPECT_VERSION` is set by the matrix and by CI. `PreCheck`
  fails when the gateway reports another version, so a stale image cannot
  masquerade as a green run.
- **Adding a release:** put it on its own line in ascending order, run the
  matrix, and land a pull request. **Dropping a release:** remove the line; if
  the floor moves, update `README.md` and the spec in the same change.
- `.github/workflows/version-freshness.yml` opens one issue labelled
  `version-matrix` when the window drifts; it never edits the file.

### The Terraform CLI in the acceptance run

`terraform-plugin-testing` drives every acceptance case through the Terraform
CLI, so a `terraform` binary must be on the `PATH` of the process that runs
`go test`. The compose `test` service (`make testacc-docker`) starts from a
bare `golang` image: it ships no Terraform CLI and cannot install one when the
container has no network.

The authoritative run is therefore on the host, against the four gateways
started by compose — `hybrid`, `readonly`, `fileowned` and `filemode`:

```sh
TF_ACC=1 \
AGENTGATEWAY_ENDPOINT=http://127.0.0.1:15000 \
AGENTGATEWAY_ENDPOINT_READONLY=http://127.0.0.1:15001 \
AGENTGATEWAY_ENDPOINT_FILEOWNED=http://127.0.0.1:15002 \
AGENTGATEWAY_ENDPOINT_FILEMODE=http://127.0.0.1:15003 \
go test -count=1 -p 1 -timeout 30m ./internal/...
```

`-p 1` is required: the gateways are shared fixtures with singleton ids
(`llm.settings/default`, `mcp.settings/default`, `llm.policy/apiKey`), so
packages must not run against them in parallel. Set
`TF_ACC_TERRAFORM_PATH=/path/to/terraform` to pin the CLI, or
`TF_ACC_TERRAFORM_VERSION=1.9.8` to have the test framework fetch that release
instead of using the one on `PATH`.

## Coverage floors

`make coverage-gate` (`tfkit coverage gate`) enforces two things on the unit
profile (`coverage.out`):

- the module total against `.github/coverage-baseline.txt`, which the README
  badge tracks;
- every package returned by `go list ./internal/...` against its entry in
  `.github/coverage-floors.txt`, or that file's `default` floor.

The package list is discovered, so a new `internal/resources/*` or
`internal/datasources/*` package is gated the moment it exists; a package with
no tests measures 0% and fails. The gate prints every package it checked and
names each one below its floor with the delta.

To **raise** a floor: improve coverage, run `make coverage`, then edit the
package's line. To **lower** one: do it in the same pull request as the change
that dropped coverage and explain why in the pull request. Lowering a floor
without that justification is a review finding. `make coverage-gate` checks
the current `coverage.out`.

## Dependency vulnerabilities

`Lint & Security` runs `govulncheck ./...` through `scripts/govulncheck.sh`.
Only findings that affect called code fail the job; module-level findings the
provider does not call are reported by the tool and do not block.

- The tool version is pinned in `.github/workflows/test.yml`
  (`golang.org/x/vuln/cmd/govulncheck@v1.9.0`); bump it deliberately together
  with the `go` directive it was tested against.
- `golangci-lint` and `gosec` are pinned and built with the job's Go toolchain
  (`install-mode: goinstall` and `go run …@version`). Released binaries are
  built with an older Go and cannot read the module's export data, so bump
  their versions together with the `go` directive.
- Fix a finding by upgrading the dependency or the Go toolchain in `go.mod`.
- If a reachable finding cannot be fixed yet, add its OSV ID and a reason to
  `.github/govulncheck-accepted.txt`. The gate prints accepted entries and warns
  when one no longer matches, so remove stale entries.

## Pull requests

- One logical change per pull request; keep the diff inside the paths the task
  owns.
- Add or update tests with the change. Unit tests live next to the code; CRUD
  tests drive the shared fake harness rather than hand-built framework values.
- Do not hand-edit `docs/`; run `make generate` and commit the result.
- `go.mod`/`go.sum` changes need the lead's approval.
- CI must be green, including the generated-doc drift check and coverage gate.

## Merging

`main` is protected by the `main-protection` repository ruleset. Do not push to
it directly: every change goes through a branch and a pull request. The ruleset
requires:

- `Lint & Security` and `Test` to pass. These are the job `name:` values in
  `.github/workflows/test.yml`, not the job keys.
- A linear history (squash merges only) and no force-pushes or branch
  deletion.
- All review threads resolved.
- No bypass actors. There is deliberately no fast path around the gate.

Merge through the merge queue once both checks are green. Repository-level
auto-merge is disabled, so `gh pr merge --auto` fails with
`Auto merge is not allowed for this repository`; enqueue the pull request with
the GraphQL `enqueuePullRequest` mutation described in
[`contributing/releasing.md`](contributing/releasing.md#2-merge-through-the-queue).
The queue builds the prospective merge commit and runs the same `pull_request`
workflows on the `merge_group` event, so stacked PRs are tested against the
latest `main` without every lane rebasing.

### Required checks and rename hazard

The ruleset policy requires exactly `ci / Lint & Security` and `ci / Test`: the
`test.yml` job id `ci`, a ` / ` separator, and the job's `name:` inside the
reusable workflow. So **renaming that `name:`, or moving the caller to another
job id, silently un-requires the gate**. Treat either as a breaking CI change:

1. Update the `name:` in the kit workflow (or the caller's job id).
2. Update `required-contexts` in `.github/workflows/protection-drift.yml` and
   this section to the contexts the workflows now report.
3. Install the new context with
   `tfkit branch-protection setup --contexts '<list>'`.
4. Verify it with the same command's `check` subcommand; it must exit 0.

Both subcommands take the comma-separated list `protection-drift.yml` passes to
`provider-protection.yml` in `required-contexts`. Run them through the kit pin
the workflows use:

```sh
go run github.com/caberdo/terraform-provider-kit/cmd/tfkit@v0.7.0 \
  branch-protection check --contexts 'ci / Lint & Security,ci / Test'
```

With no `--contexts` tfkit falls back to `Lint & Security,Test`, which is not a
job name here: `check` reports that as drift, and `setup` refuses it before it
writes anything.

Never require a job that has an `if:` condition or a matrix context. Such a
check is skipped on some PRs and sits at "Expected — Waiting for status to be
reported" forever. `Prepare agentgateway Matrix`, `Acceptance Tests (agentgateway
<version>)` and `Check the agentgateway release window` are therefore not
required. Do not add `paths:` or `branches:` filters to a workflow whose jobs
are required.

`tfkit branch-protection check --contexts 'ci / Lint & Security,ci / Test'` is
read-only and exits non-zero on drift. `.github/workflows/protection-drift.yml`
runs it weekly through the kit's `provider-protection.yml` and opens (or updates)
one issue labelled `ci`.

### Emergency procedure

If `main` must be fixed while the gate is red, disable the ruleset explicitly
(`tfkit branch-protection setup --contexts 'ci / Lint & Security,ci / Test'`
installs it with `enforcement: active`; set it to `disabled` in repository
settings or via the API), land the smallest possible hotfix through a PR, then
re-enable it by running that same setup command again. A visible disable/re-enable
window is the documented path; do not add a bypass actor for convenience.

### TODO: second maintainer

`required_approving_review_count` is `0` because GitHub does not allow
self-approval and there is one maintainer today. When a second maintainer is
added, raise it to `1` and set `require_code_owner_review: true` in the same
change: `tfkit branch-protection setup` hardcodes both, so that change lands in
the kit and is installed here by re-running the setup command.

## Releasing

A release is a pull request that moves `VERSION`, cuts the `CHANGELOG.md`
`Unreleased` section and updates the README install constraint. Merging it
starts the `Create Release Tag` workflow, which signs the tag and calls
`Release`; a merge that leaves `VERSION` alone never releases.

The manual `Create Release Tag` dispatch remains for the cases the merge cannot
cover, and `Release` with an existing tag republishes a failed one. The full
path, including verification and recovery, is in
[`contributing/releasing.md`](contributing/releasing.md). Do not re-run
`Create Release Tag` to recover a failed publish.

## Commits

Use conventional commit prefixes:

- `feat(<area>): ...` for user-visible behaviour
- `fix(<area>): ...` for bug fixes
- `chore: ...` for tooling, CI and scaffolding
- `docs: ...` for documentation-only changes
- `test: ...` for test-only changes

## Reporting bugs and requesting features

Use the issue templates. For a bug, include the provider version, Terraform
version, agentgateway version, the configuration that triggered it, and the
relevant diagnostic with secrets removed.
