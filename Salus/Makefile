SHELL := /bin/bash

# Usage:
#   make build | test | vet | lint | fmt | cover
#   make release VERSION=v0.1.0
#   make tag VERSION=v0.1.0
#   make push-tag VERSION=v0.1.0

# CI pins golangci-lint v2.13.2; override the command with GOLANGCI_LINT=...
GOLANGCI_LINT ?= golangci-lint

.PHONY: help build test vet lint fmt cover check-version tag push-tag release

help:
	@echo "Targets:"
	@echo "  make build                    Build ./salus (needs a C toolchain for CGO)"
	@echo "  make test                     Run the tests"
	@echo "  make vet                      Run go vet"
	@echo "  make lint                     Run golangci-lint for linux, darwin, and windows"
	@echo "  make fmt                      Format Go sources with gofmt -s"
	@echo "  make cover                    Write coverage.out and print coverage per function"
	@echo "  make tag VERSION=vX.Y.Z       Create annotated git tag"
	@echo "  make push-tag VERSION=vX.Y.Z  Push tag to origin"
	@echo "  make release VERSION=vX.Y.Z   Create and push tag (triggers CD)"

# go-sqlite3 needs CGO; a CGO_ENABLED=0 build compiles but cannot open a database.
build:
	CGO_ENABLED=1 go build -o salus .

test:
	go test ./...

vet:
	go vet ./...

# CI lints on Linux, macOS, and Windows. Each GOOS is explicit so every host
# lints all three; cross-OS linting needs no C toolchain.
lint:
	GOOS=linux $(GOLANGCI_LINT) run ./...
	GOOS=darwin $(GOLANGCI_LINT) run ./...
	GOOS=windows $(GOLANGCI_LINT) run ./...

fmt:
	gofmt -s -w .

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

check-version:
	@if [[ -z "$(VERSION)" ]]; then \
		echo "ERROR: VERSION is required (example: VERSION=v0.1.0)"; \
		exit 1; \
	fi
	@if [[ ! "$(VERSION)" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z\.-]+)?$$ ]]; then \
		echo "ERROR: VERSION must look like vMAJOR.MINOR.PATCH (example: v1.2.3)"; \
		exit 1; \
	fi

tag: check-version
	@git rev-parse --is-inside-work-tree >/dev/null
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "ERROR: tag $(VERSION) already exists locally"; \
		exit 1; \
	fi
	git tag -a "$(VERSION)" -m "Release $(VERSION)"
	@echo "Created tag $(VERSION)"

push-tag: check-version
	@git rev-parse --is-inside-work-tree >/dev/null
	@if ! git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "ERROR: tag $(VERSION) does not exist locally. Run: make tag VERSION=$(VERSION)"; \
		exit 1; \
	fi
	git push origin "$(VERSION)"
	@echo "Pushed tag $(VERSION)"

release: tag push-tag
	@echo "Release tag $(VERSION) created and pushed."
