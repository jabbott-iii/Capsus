SHELL := /bin/bash

# Capsus contributor tasks. `make check` runs what CI runs (ci.yml) in every Go
# module of the repository: the launcher at the root and the imported tools in
# Munus/, Cryptare/ and Salus/ (git subtrees with their own go.mod; see
# intel/maint.md). `make check-all` adds the security scans (security.yml) and
# the workflow linter. Run `make` or `make help` for the list. The recipes need
# bash, git, Go 1.26 and a C compiler (go-sqlite3 needs CGO); docker-static
# needs Docker. Until intel/plan.md Phases 1-2 land, every target that needs
# the modules stops at the `layout` check.
#
# Release tagging:
#   make release VERSION=v0.1.0
#   make tag VERSION=v0.1.0
#   make push-tag VERSION=v0.1.0

GO ?= go

# The imported tools, and every module the checks run in. `go ... ./...` never
# enters a nested module, so the targets below loop over MODULES.
TOOLS := Munus Cryptare Salus
MODULES := . $(TOOLS)

# go-sqlite3 needs CGO: a CGO_ENABLED=0 build compiles but cannot open a
# database. The race detector needs it too. Set per command rather than
# exported, so tools run through `make tool` keep their own defaults.
CGO ?= 1

# The version stamped into local builds; `capsus --version` prints it. VERSION
# is kept for the release targets, so local builds use their own variable.
BUILD_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN_DIR ?= bin
BINARY := $(BIN_DIR)/capsus$(shell $(GO) env GOEXE 2>/dev/null)

# Tool versions, kept in step with the workflows: golangci-lint (ci.yml), gosec
# and govulncheck (security.yml). `go run pkg@version` runs exactly these
# versions, verified by the Go checksum database. To use an installed binary
# instead: make lint GOLANGCI_LINT=golangci-lint
GOLANGCI_LINT_VERSION := v2.13.2
GOSEC_VERSION := v2.29.0
GOVULNCHECK_VERSION := v1.8.0
ACTIONLINT_VERSION := v1.7.12
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOSEC ?= $(GO) run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)
GOVULNCHECK ?= $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
ACTIONLINT ?= $(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

# The launcher's own Go files: everything outside the tool folders. `make fmt`
# rewrites only these; tool code changes upstream (intel/maint.md §3).
ROOT_GO_FILES = $(shell find . \( $(foreach t,$(TOOLS),-path ./$(t) -o) -path ./.git \) -prune -o -type f -name '*.go' -print)

# Targets of the tools' own Makefiles that `make tool` may run; not every tool
# has every target (`make tool TOOL=<Tool> TARGET=help` lists them). Release
# targets (tag, push-tag, release) are left out on purpose: inside this
# repository they would tag and push Capsus, not the tool (intel/cybersec.md
# SEC-001). So are targets that write into the tool folder's tracked content
# (fmt, licenses, and fuzz, which saves failing inputs under testdata/).
TOOL_TARGETS := help build test race test-race cover vet fmt-check lint tidy-check keys-check golden smoke check sec vuln security

# each_module runs $(1) in every module directory, stopping at the first
# failure. $(1) must not contain a comma ($(call) splits arguments at commas).
define each_module
set -e; for m in $(MODULES); do echo "== module: $$m"; (cd "$$m" && { $(1); }); done
endef

.DEFAULT_GOAL := help

.PHONY: help layout build smoke docker-static clean test race cover vet fmt fmt-check lint \
	tidy-check keys-check deps-drift check sec vuln actionlint security check-all tool \
	check-version tag push-tag release

##@ General

help: ## Show this help
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@ / { printf "\n%s\n", substr($$0, 5); next } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  make %-14s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf '\nModules: %s\nVariables: BUILD_VERSION=%s TOOL=%s TARGET=%s\n' \
		'$(MODULES)' '$(BUILD_VERSION)' '$(TOOL)' '$(TARGET)'

layout: ## Fail unless the root module and every tool module are present (intel/plan.md)
	@missing=0; \
	for m in $(MODULES); do \
		if [[ ! -f "$$m/go.mod" ]]; then echo "missing: $$m/go.mod"; missing=1; fi; \
	done; \
	if (( missing )); then \
		echo "The tools are not fully imported yet; see intel/plan.md (Phase 1: git subtree add, Phase 2: root go.mod)."; \
		exit 1; \
	fi

##@ Build

build: layout ## Build bin/capsus with CGO, stamped with BUILD_VERSION
	CGO_ENABLED=$(CGO) $(GO) build -trimpath -ldflags "-X main.version=$(BUILD_VERSION)" -o $(BINARY) .

smoke: build ## Smoke-test bin/capsus as CI does (scripts/smoke.sh), with databases in a temporary folder
	bash scripts/smoke.sh "$(BINARY)" "$(BUILD_VERSION)"

docker-static: ## Build the static Linux release binary as cd.yml does, into bin/static/ (needs the Dockerfile, plan P-030)
	@if [[ ! -f Dockerfile ]]; then echo "No Dockerfile yet; see intel/plan.md P-030."; exit 1; fi
	docker build --target static --build-arg VERSION="$(BUILD_VERSION)" --output "type=local,dest=$(BIN_DIR)/static" .

clean: ## Remove bin/ and every module's coverage.out
	rm -rf $(BIN_DIR)
	@for m in $(MODULES); do rm -f "$$m/coverage.out"; done

##@ Test (every module)

test: layout ## Run the tests in every module
	@$(call each_module,CGO_ENABLED=$(CGO) $(GO) test ./...)

race: layout ## Run the tests with the race detector, uncached (as CI does on Linux and macOS)
	@$(call each_module,CGO_ENABLED=$(CGO) $(GO) test -race -count=1 ./...)

cover: layout ## Write coverage.out in every module and print each module's total
	@$(call each_module,CGO_ENABLED=$(CGO) $(GO) test -coverprofile=coverage.out ./... && $(GO) tool cover -func=coverage.out | tail -n 1)

##@ Code quality (CI: ci.yml)

fmt: ## Format the launcher's Go files with gofmt -s (tool folders are never rewritten)
	@files="$(ROOT_GO_FILES)"; \
	if [[ -n "$$files" ]]; then gofmt -s -w $$files; else echo "No launcher Go files yet."; fi

fmt-check: layout ## Fail if any Go file in any module needs gofmt -s
	@files="$$(gofmt -s -l $(ROOT_GO_FILES) $(TOOLS))"; \
	if [[ -n "$$files" ]]; then \
		echo "These files need gofmt -s (launcher: make fmt; tool folders: fix upstream):"; \
		echo "$$files"; \
		exit 1; \
	fi

vet: layout ## Run go vet in every module
	@$(call each_module,CGO_ENABLED=$(CGO) $(GO) vet ./...)

lint: layout ## Run golangci-lint (CI's version) in every module, with each module's own configuration
	@$(call each_module,$(GOLANGCI_LINT) run ./...)

tidy-check: layout ## Fail if go mod tidy would change any go.mod or go.sum (changes nothing itself)
	@$(call each_module,$(GO) mod tidy -diff)

keys-check: ## Fail if a database or key export is tracked by git anywhere (intel/cybersec.md)
	@if git ls-files | grep -iE '\.(db|db-journal|db-wal|db-shm|ckey)$$'; then \
		echo "Databases or key exports are tracked. Remove them with 'git rm --cached'."; \
		exit 1; \
	fi

deps-drift: layout ## Report modules the capsus build uses at a different version than a tool's go.mod (informational)
	@set -eo pipefail; \
	shipped="$$($(GO) list -m -f '{{if not .Main}}{{.Path}} {{.Version}}{{end}}' all)"; \
	drift=0; \
	for t in $(TOOLS); do \
		wanted="$$(cd "$$t" && $(GO) list -m -f '{{if not .Main}}{{.Path}} {{.Version}}{{end}}' all)"; \
		while read -r path ver; do \
			[[ -n "$$path" ]] || continue; \
			got="$$(awk -v p="$$path" '$$1 == p { print $$2 }' <<<"$$shipped")"; \
			if [[ -n "$$got" && "$$got" != "$$ver" ]]; then \
				echo "$$t: $$path $$ver in $$t/go.mod, $$got in the capsus build"; drift=1; \
			fi; \
		done <<<"$$wanted"; \
	done; \
	if (( drift )); then \
		echo "Those tools' tests ran other versions than capsus ships (intel/plan.md R-1)."; \
	else \
		echo "No drift: every tool's go.mod matches the capsus build."; \
	fi

check: layout keys-check fmt-check tidy-check vet lint race smoke ## Run everything CI runs, in CI's order
	@echo "All CI checks passed."

##@ Security (CI: security.yml)

sec: layout ## Run gosec (security.yml's version) in every module; fails after scanning all if any has findings
	@failed=""; \
	for m in $(MODULES); do \
		echo "== module: $$m"; \
		(cd "$$m" && $(GOSEC) -quiet ./...) || failed="$$failed $$m"; \
	done; \
	if [[ -n "$$failed" ]]; then echo "gosec reported findings in:$$failed"; exit 1; fi

vuln: layout ## Run govulncheck on the root module: the build list and call graph that ship (intel/cybersec.md SEC-002)
	$(GOVULNCHECK) ./...

actionlint: ## Lint the GitHub workflows (uses shellcheck too, when it is installed)
	$(ACTIONLINT)

security: sec vuln ## Run gosec and govulncheck

check-all: check security actionlint ## Run CI's checks, the security scans and the workflow linter: the full pre-PR check
	@echo "All checks passed."

##@ Tools

tool: layout ## Run one of a tool's own Makefile targets: make tool TOOL=Cryptare TARGET=golden
	@if [[ " $(TOOLS) " != *" $(TOOL) "* ]]; then echo "TOOL must be one of: $(TOOLS)"; exit 1; fi
	@if [[ " $(TOOL_TARGETS) " != *" $(TARGET) "* ]]; then \
		echo "TARGET must be one of: $(TOOL_TARGETS)"; \
		echo "(Release targets and targets that rewrite files are not allowed here; see intel/cybersec.md SEC-001.)"; \
		exit 1; \
	fi
	$(MAKE) -C "$(TOOL)" $(TARGET)

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
