default: build

BINARY_NAME=terraform-provider-agentgateway
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
# agentgateway server version the acceptance tests run against; docker-compose.yml
# reads the same variable. Override on the command line to test another version.
AGENTGATEWAY_VERSION ?= v1.7.0
# Compose implementation. Override for Podman:
#   make testacc-docker COMPOSE=podman-compose
COMPOSE ?= docker compose
# Supported versions, single-sourced with the CI matrix. Override for a quick
# local check: AGENTGATEWAY_VERSIONS="v1.7.0 v1.6.0" make testacc-matrix
AGENTGATEWAY_VERSIONS ?= $(shell sed -e 's/\#.*//' .github/agentgateway-versions.txt | tr '\n' ' ')
# The shared tooling for the caberdo providers: the coverage gate, merge and
# README targets, example validation and the release-metadata check. Keep this
# pin equal to the `uses:` refs in .github/workflows/*.yml: one version for the
# local targets and the reusable workflows, so they cannot drift apart.
TFKIT ?= go run github.com/caberdo/terraform-provider-kit/cmd/tfkit@v0.8.0

build:
	go build -o bin/$(BINARY_NAME) -ldflags "-X main.version=$(VERSION)" .

test:
	go test -v -cover ./internal/...

# Unit coverage only: `TestAcc*` needs TF_ACC and a reachable agentgateway,
# so it is skipped to keep the profile independent of the environment.
coverage:
	go test -count=1 -skip '^TestAcc' -covermode=atomic -coverprofile=coverage.out ./internal/...
	@echo
	@go tool cover -func=coverage.out

coverage-html: coverage
	go tool cover -html=coverage.out

# Unit coverage (coverage.out) plus the acceptance profile written by
# docker-compose.yml (coverage-acceptance.out) as one number.
coverage-merge:
	$(TFKIT) coverage merge --output coverage-union.out coverage.out coverage-acceptance.out
	@echo
	@go tool cover -func=coverage-union.out

# Regenerate the coverage badge and floor statement in README.md from
# .github/coverage-baseline.txt; `make coverage-readme-check` fails when the
# committed README has drifted from the baseline.
coverage-readme:
	$(TFKIT) coverage readme

coverage-readme-check:
	$(TFKIT) coverage readme --check

# Fail when coverage.out (unit + protocol) is below the floor in
# .github/coverage-baseline.txt. Acceptance coverage is reported separately and
# is not gated: it depends on Docker being available (design §10).
coverage-gate:
	$(TFKIT) coverage gate --profile coverage.out

testacc:
	TF_ACC=1 go test -v -cover ./internal/...

testacc-docker:
	AGENTGATEWAY_VERSION=$(AGENTGATEWAY_VERSION) $(COMPOSE) run --rm test

# Run the docker-based acceptance suite against every supported agentgateway
# version. Each version runs and tears down independently so one failure still
# lets the others run; the overall exit status is non-zero if any version failed.
#
# The project name rewrites dots to dashes: Compose rejects them in a project
# name, and the kit's acceptance workflow builds the same name the same way, so
# local and CI matrices stay tellable apart as `agentgateway-acc-v1-7-0`.
testacc-matrix:
	@test -n "$(AGENTGATEWAY_VERSIONS)" || { \
		echo "AGENTGATEWAY_VERSIONS is empty; check .github/agentgateway-versions.txt" >&2; \
		exit 1; \
	}
	@results=$$(mktemp); status=0; \
	for v in $(AGENTGATEWAY_VERSIONS); do \
		project="agentgateway-acc-$$(echo "$$v" | tr . -)"; \
		echo "==> Acceptance tests against agentgateway $$v"; \
		if COMPOSE_PROJECT_NAME=$$project AGENTGATEWAY_EXPECT_VERSION=$$v \
			$(MAKE) --no-print-directory testacc-docker AGENTGATEWAY_VERSION=$$v; then \
			echo "PASS agentgateway $$v" >> "$$results"; \
		else \
			echo "FAIL agentgateway $$v" >> "$$results"; status=1; \
		fi; \
		COMPOSE_PROJECT_NAME=$$project $(COMPOSE) down -v >/dev/null 2>&1 || true; \
	done; \
	echo; echo "==> Acceptance matrix summary"; cat "$$results"; rm -f "$$results"; \
	exit $$status

# Fail when a tracked Go file is not gofmt-formatted. gofmt is not one of
# golangci-lint's default linters, so it gets its own gate. CI runs the same
# check through `gofmt-check: true` in provider-ci.yml; this local target catches
# a formatting slip before a push.
fmt-check:
	@files=$$(git ls-files '*.go'); \
	if [ -z "$$files" ]; then exit 0; fi; \
	unformatted=$$(gofmt -l $$files); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt would reformat these files:" >&2; \
		echo "$$unformatted" >&2; \
		echo "run 'gofmt -w' on them (or 'make fmt')" >&2; \
		exit 1; \
	fi

lint: fmt-check
	golangci-lint run --config .golangci.yml ./...

lint-fix:
	golangci-lint run --config .golangci.yml --fix ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/caberdo/agentgateway/$(VERSION)/$$(go env GOOS)_$$(go env GOARCH)
	cp bin/$(BINARY_NAME) ~/.terraform.d/plugins/registry.terraform.io/caberdo/agentgateway/$(VERSION)/$$(go env GOOS)_$$(go env GOARCH)/$(BINARY_NAME)

validate-examples:
	$(TFKIT) examples validate --provider agentgateway

generate:
	go generate ./...

# Fail when the release metadata a release pull request carries is inconsistent.
# The tag workflow enforces the same agreement on main; this keeps the failure
# in the pull request instead of after the merge.
release-check:
	$(TFKIT) release check

.PHONY: default build test coverage coverage-html coverage-merge coverage-readme coverage-readme-check coverage-gate testacc testacc-docker testacc-matrix lint lint-fix fmt fmt-check vet install validate-examples generate release-check
