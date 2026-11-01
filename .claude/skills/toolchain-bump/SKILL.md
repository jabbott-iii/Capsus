---
name: toolchain-bump
description: Move Cryptare to a new Go toolchain release (for example go1.26.10), keeping go.mod, the Dockerfile builder image and cd.yml's LINUX_BUILDER in step, and updating the docs that name the version.
disable-model-invocation: true
argument-hint: "<go version, e.g. go1.26.10>"
---

# Go toolchain bump to $ARGUMENTS

1. **Check the request.** Current version: the `toolchain` line in `go.mod`. A patch
   bump (1.26.x) is routine. A minor bump (1.27) is a deliberate change: confirm with
   the owner first, and treat a Unicode-table change as a possible format change
   (`TestNormalizePasswordKnownAnswers`; `/format-change`).

2. **Find the image digest.** The builder is `golang:<ver>-alpine` pinned by the
   multi-platform index digest. Get it from the registry, for example
   `docker buildx imagetools inspect golang:<ver>-alpine` (the top-level `Digest`).
   Never guess or reuse a digest; if you can't look it up, stop and ask the user for it.

3. **Edit together**, nothing else:
   - `go.mod`: `toolchain go<ver>` (leave the `go 1.26.0` line unless the owner asked
     for a minor bump);
   - `Dockerfile`: `FROM golang:<ver>-alpine@sha256:<digest> AS builder`;
   - `.github/workflows/cd.yml`: `LINUX_BUILDER: golang:<ver>-alpine@sha256:<digest>`;
   - `README.md` Prerequisites and `CONTRIBUTING.md` Prerequisites, where they name
     the selected version.

4. **Find stragglers.** `bash .claude/skills/doc-audit/check-versions.sh`, then grep
   the old version string across the repository; every remaining hit should be
   historical (upgrade notes, comments about past releases).

5. **Validate** what the environment allows: `make check-all` (govulncheck checks the
   new toolchain's standard library), `make docker-smoke` if Docker is available.
   Report anything you couldn't run.

6. **Report** the files changed, the digest and where it came from, validation
   results, and the reason for the bump (for example the advisory it fixes, at summary
   level).
