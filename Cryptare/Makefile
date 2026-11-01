SHELL := /bin/bash

# Cryptare's contributor tasks. `make check` runs what CI runs (ci.yml); `make check-all`
# adds the security scans (security.yml) and the workflow linter. Run `make` or
# `make help` for the list. The recipes need bash, Go (go.mod selects the toolchain) and
# a C compiler for CGO; the docker-* targets need Docker. Works with the GNU Make 3.81
# that macOS ships.
#
# Release tagging (unchanged):
#   make release VERSION=v0.1.0
#   make tag VERSION=v0.1.0
#   make push-tag VERSION=v0.1.0

GO ?= go

# go-sqlite3 needs CGO: a CGO_ENABLED=0 build compiles, but can't open the key store.
# The race detector needs it too.
CGO ?= 1

# The version stamped into local builds; `cryptare --version` prints it. VERSION is kept
# for the release targets, so local builds use their own variable.
BUILD_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/cryptare$(shell $(GO) env GOEXE 2>/dev/null)

# Tool versions, kept in step with the workflows: golangci-lint (ci.yml), gosec and
# govulncheck (security.yml), actionlint (CI doesn't run it). `go run pkg@version` uses
# exactly these, verified by the Go checksum database, with nothing to install. To use
# a binary you installed instead: make lint GOLANGCI_LINT=golangci-lint
GOLANGCI_LINT_VERSION := v2.13.2
GOSEC_VERSION := v2.29.0
GOVULNCHECK_VERSION := v1.8.0
ACTIONLINT_VERSION := v1.7.12
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOSEC ?= $(GO) run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
GOVULNCHECK ?= $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
ACTIONLINT ?= $(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# Fuzzing: one target per run (Go's limit). The targets are in pkg/fuzz_test.go:
# FuzzParseV2Header, FuzzDecryptingReader, FuzzKeyExportValidation, FuzzParseSize,
# FuzzExtractTar and FuzzExtractZip. A short minimisation time keeps the extraction
# fuzzers from appearing to hang.
FUZZ ?= FuzzParseV2Header
FUZZTIME ?= 1m

DOCKER_IMAGE ?= cryptare:dev

.DEFAULT_GOAL := help

.PHONY: help build test test-race cover cover-html golden fuzz fmt fmt-check vet lint \
	tidy-check keys-check smoke sec vuln actionlint docker-build docker-smoke check \
	security check-all clean check-version tag push-tag release

##@ General

help: ## Show this help
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@ / { printf "\n%s\n", substr($$0, 5); next } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  make %-13s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf '\nVariables: BUILD_VERSION=%s FUZZ=%s FUZZTIME=%s DOCKER_IMAGE=%s\n' \
		'$(BUILD_VERSION)' '$(FUZZ)' '$(FUZZTIME)' '$(DOCKER_IMAGE)'

##@ Build

build: ## Build bin/cryptare with CGO, stamped with BUILD_VERSION
	CGO_ENABLED=$(CGO) $(GO) build -trimpath -ldflags "-X main.version=$(BUILD_VERSION)" -o $(BINARY) .

smoke: build ## Run CI's smoke test on bin/cryptare, plus a stored-key round trip, in a temporary folder
	@set -euo pipefail; \
	tmp="$$(mktemp -d)"; trap 'rm -rf "$$tmp"' EXIT; \
	bin="$(CURDIR)/$(BINARY)"; \
	export CRYPTARE_DB_PATH="$$tmp/smoke.db"; \
	ver="$$("$$bin" --version)"; grep -qF -- "$(BUILD_VERSION)" <<<"$$ver"; \
	printf 'make-smoke-passphrase\n' > "$$tmp/pw.txt"; \
	printf 'cryptare make smoke test\n' > "$$tmp/smoke.txt"; \
	"$$bin" keys generate --password-file "$$tmp/pw.txt" >/dev/null; \
	keys="$$("$$bin" keys list)"; grep -q "AES-256-GCM" <<<"$$keys"; \
	"$$bin" encrypt "$$tmp/smoke.txt" --password-file "$$tmp/pw.txt" >/dev/null; \
	"$$bin" decrypt "$$tmp/smoke.txt.enc" --output "$$tmp/smoke.out" --password-file "$$tmp/pw.txt" >/dev/null; \
	cmp "$$tmp/smoke.txt" "$$tmp/smoke.out"; \
	key="$$(awk 'NR == 2 { print $$1 }' <<<"$$keys")"; \
	"$$bin" encrypt "$$tmp/smoke.txt" --output "$$tmp/key.enc" --key "$$key" --password-file "$$tmp/pw.txt" >/dev/null; \
	"$$bin" decrypt "$$tmp/key.enc" --output "$$tmp/key.out" --password-file "$$tmp/pw.txt" >/dev/null; \
	cmp "$$tmp/smoke.txt" "$$tmp/key.out"; \
	echo "Smoke test passed: $$ver"

clean: ## Remove bin/ and coverage output
	rm -rf $(BIN_DIR) coverage.out coverage.html

##@ Test

test: ## Run the tests
	CGO_ENABLED=$(CGO) $(GO) test ./...

test-race: ## Run the tests with the race detector, uncached (as CI does on Linux and macOS)
	CGO_ENABLED=$(CGO) $(GO) test -race -count=1 ./...

cover: ## Run the tests with the race detector and write coverage.out; print the total
	CGO_ENABLED=$(CGO) $(GO) test -race -count=1 -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -n 1

cover-html: cover ## Write the coverage report to coverage.html
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

golden: ## Run only the golden-fixture tests (on-disk format compatibility)
	CGO_ENABLED=$(CGO) $(GO) test -count=1 -run '^TestGolden' ./pkg

fuzz: ## Fuzz one target: make fuzz FUZZ=FuzzExtractTar FUZZTIME=5m
	CGO_ENABLED=$(CGO) $(GO) test -run '^$$' -fuzz '^$(FUZZ)$$' -fuzztime $(FUZZTIME) -fuzzminimizetime 3s ./pkg

##@ Code quality (CI: ci.yml)

fmt: ## Format the code with gofmt -s
	gofmt -s -w .

fmt-check: ## Fail if any file needs gofmt -s
	@files="$$(gofmt -s -l .)"; \
	if [ -n "$$files" ]; then echo "These files need formatting (run make fmt):"; echo "$$files"; exit 1; fi

vet: ## Run go vet
	CGO_ENABLED=$(CGO) $(GO) vet ./...

lint: ## Run golangci-lint (CI's version)
	$(GOLANGCI_LINT) run ./...

tidy-check: ## Fail if go mod tidy would change go.mod or go.sum (changes nothing itself)
	$(GO) mod tidy -diff

keys-check: ## Fail if a key database or key export is tracked by git (SEC-003)
	@if git ls-files | grep -iE '\.(db|db-journal|db-wal|db-shm|ckey)$$'; then \
		echo "Key databases or key exports are tracked. Remove them with 'git rm --cached' (SEC-003)."; \
		exit 1; \
	fi

check: fmt-check tidy-check keys-check vet lint test-race smoke ## Run everything CI runs, in CI's order
	@echo "All CI checks passed."

##@ Security (CI: security.yml)

sec: ## Run gosec (security.yml's version)
	$(GOSEC) -quiet ./...

vuln: ## Run govulncheck: fails if the code reaches a known vulnerability
	$(GOVULNCHECK) ./...

actionlint: ## Lint the GitHub workflows (uses shellcheck too, when it is installed)
	$(ACTIONLINT)

security: sec vuln ## Run gosec and govulncheck

check-all: check security actionlint ## Run CI's checks, the security scans and the workflow linter: the full pre-PR check
	@echo "All checks passed."

##@ Docker (CI: docker.yml)

docker-build: ## Build the image as DOCKER_IMAGE (default cryptare:dev), stamped with BUILD_VERSION
	docker build --build-arg VERSION="$(BUILD_VERSION)" --tag "$(DOCKER_IMAGE)" --file Dockerfile .

docker-smoke: docker-build ## Smoke-test the image as docker.yml does: UID 10001, version, key store on a throwaway volume
	@set -euo pipefail; \
	vol="cryptare-make-smoke-$$$$"; tmp="$$(mktemp -d)"; \
	trap 'docker volume rm -f "$$vol" >/dev/null 2>&1 || true; rm -rf "$$tmp"' EXIT; \
	test "$$(docker run --rm --entrypoint id "$(DOCKER_IMAGE)" -u)" = 10001; \
	ver="$$(docker run --rm "$(DOCKER_IMAGE)" --version)"; grep -qF -- "$(BUILD_VERSION)" <<<"$$ver"; \
	printf 'docker-smoke-passphrase\n' > "$$tmp/pw.txt"; chmod 644 "$$tmp/pw.txt"; \
	docker volume create "$$vol" >/dev/null; \
	docker run --rm -v "$$vol:/app/data" -v "$$tmp/pw.txt:/run/pw.txt:ro" "$(DOCKER_IMAGE)" \
		keys generate --password-file /run/pw.txt >/dev/null; \
	keys="$$(docker run --rm -v "$$vol:/app/data" "$(DOCKER_IMAGE)" keys list)"; grep -q "AES-256-GCM" <<<"$$keys"; \
	echo "Docker smoke test passed: $(DOCKER_IMAGE)"

##@ Release (maintainer)

check-version:
	@if [[ -z "$(VERSION)" ]]; then \
		echo "ERROR: VERSION is required (example: VERSION=v0.1.0)"; \
		exit 1; \
	fi
	@if [[ ! "$(VERSION)" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z\.-]+)?$$ ]]; then \
		echo "ERROR: VERSION must look like vMAJOR.MINOR.PATCH (example: v1.2.3)"; \
		exit 1; \
	fi

tag: check-version ## Create annotated tag VERSION=vX.Y.Z
	@git rev-parse --is-inside-work-tree >/dev/null
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "ERROR: tag $(VERSION) already exists locally"; \
		exit 1; \
	fi
	git tag -a "$(VERSION)" -m "Release $(VERSION)"
	@echo "Created tag $(VERSION)"

push-tag: check-version ## Push tag VERSION=vX.Y.Z to origin
	@git rev-parse --is-inside-work-tree >/dev/null
	@if ! git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "ERROR: tag $(VERSION) does not exist locally. Run: make tag VERSION=$(VERSION)"; \
		exit 1; \
	fi
	git push origin "$(VERSION)"
	@echo "Pushed tag $(VERSION)"

release: tag push-tag ## Create and push tag VERSION=vX.Y.Z (triggers CD)
	@echo "Release tag $(VERSION) created and pushed."
