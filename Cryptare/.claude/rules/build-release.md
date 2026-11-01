---
paths:
  - "go.mod"
  - "go.sum"
  - "Makefile"
  - "Dockerfile"
  - ".dockerignore"
  - ".github/**"
  - "scripts/**"
  - ".devcontainer/**"
---

# Build, CI, release and dependencies

Change these only when the task needs it, and say why in the completion report.

## Things that must move together

- **Go toolchain**: `go.mod` `toolchain` line, the `Dockerfile` builder
  `golang:<ver>-alpine@sha256:<digest>`, and `LINUX_BUILDER` in `.github/workflows/cd.yml`
  (same image and digest). The Linux release binaries are built inside that image,
  which uses its own Go (the official golang image sets `GOTOOLCHAIN=local`) whatever
  `go.mod`'s toolchain line says, so a mismatch does not fail anything: keep the three
  in step by hand. README and CONTRIBUTING name the version too. Use `/toolchain-bump`.
- **Tool versions**: golangci-lint (`Makefile` and `ci.yml`), gosec and govulncheck
  (`Makefile` and `security.yml`), actionlint (`Makefile`), and the versions listed in
  CONTRIBUTING's prerequisites.
- **Release targets**: the `cd.yml` build matrix and the default `targets` list in
  `scripts/third-party-licenses.sh`, plus README's "Release archives" table.
- **Smoke tests**: `make smoke` runs CI's smoke step plus a version check and a
  stored-key round trip; `make docker-smoke` runs `docker.yml`'s UID and key-store
  checks (with `--version` where `docker.yml` runs `--help`). Keep them at least as
  strict as CI, and keep the smoke steps in CI and CD: they are what catches a CGO-less
  build.

## Rules

- Third-party actions are pinned to a full commit SHA with the version in a comment;
  base images are pinned by digest. Never replace a pin with a tag.
- Workflows keep least-privilege `permissions:` (top level `contents: read`;
  `security.yml` adds `security-events: write` for Code Scanning uploads). In
  `cd.yml`, only the `release` and `image-attest` jobs have `id-token: write` and
  `attestations: write`, and only the `image` job has `packages: write`.
- The container runs as UID/GID 10001, which owns `/app/data`; `docker.yml` and CD
  check the UID. Don't add runtime packages.
- `main.version` is set with `-ldflags "-X main.version=<tag>"`; keep its name and
  package, which CD depends on.
- **Dependencies.** Add one only when the standard library or an existing dependency
  can't do the job, and only with the owner's agreement. Afterwards:
  `make tidy-check` must pass with the intended `go.mod`/`go.sum` change; run
  `scripts/third-party-licenses.sh`, and add a module that ships no licence file to its
  `stated_licence` list only after checking its licence (the release fails until then).
  A `golang.org/x/text` update that breaks `TestNormalizePasswordKnownAnswers` is a
  format change (`/format-change`).
- Dependabot proposes Go module, action and Docker updates weekly (golang patch versions
  only). Apply a golang image update to all three toolchain places together.
- Releases are cut by the owner pushing a `vX.Y.Z` tag (`make release VERSION=…`),
  which runs `cd.yml`. Never tag or push; `/release-prep` prepares and reports.
- After changing a workflow, run `make actionlint`. After changing the Makefile, check
  that `make help` still lists every public target (a target appears only with a
  `## description`).
