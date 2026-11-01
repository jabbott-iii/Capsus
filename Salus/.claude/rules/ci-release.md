---
paths:
  - ".github/**"
  - "Dockerfile"
  - ".dockerignore"
  - "Makefile"
  - ".golangci.yml"
  - "go.mod"
  - "go.sum"
---

# CI, CD, container image, and dependencies

Change these only when the task requires it. Pull-request checks do not
exercise most of `cd.yml`, so mistakes there surface at release time.

## Editing workflow files

- `ci.yml`, `cd.yml`, and `security.yml` (and `NOTICE`) use CRLF line
  endings; `docker.yml` uses LF. Preserve each file's line endings.
- Pin every third-party action by full commit SHA with a version comment
  (`uses: owner/action@<sha> # vX.Y.Z`), including actions you add.
  Downloads in workflows are pinned by checksum (see the llvm-mingw step).
- Bump coupled actions together, in one change:
  - every `github/codeql-action/*` sub-action (a split bump fails CodeQL);
  - `actions/upload-artifact` and `actions/download-artifact`.
  Dependabot groups both sets (`.github/dependabot.yml`).
- Dependabot cannot see versions pinned in `run:` steps or `with:` inputs:
  `GOVULNCHECK_VERSION` in `security.yml` and the golangci-lint `version:`
  in `ci.yml`. When you bump golangci-lint, also update the `Makefile`
  comment, `README.md`, and `CONTRIBUTING.md`. The version in the
  `.golangci.yml` comments records where the linter set came from; leave it.
- Keep the triggers: CI and Security on push and pull request to every
  branch, Docker on `main` and pull requests to `main`, CD on `v*` tags and
  `workflow_dispatch`.
- Keep runner labels: CD pins `ubuntu-24.04` and `ubuntu-24.04-arm` so
  release builds change OS only deliberately; CI, Security, and Docker stay
  on `ubuntu-latest`.
- Smoke tests use the real binary name, `SALUS_DB_PATH`, and real Salus
  commands; they accept `check run` exit codes `0` and `1` only (runner
  state varies) and assert persistence with `jobs show`.
- govulncheck is blocking and gosec runs with `-no-fail` (findings are
  triaged in GitHub Code Scanning). Do not make a blocking check
  non-blocking or remove a scanner.

## Permissions

- Every workflow defaults to `contents: read`. Grant a write permission only
  to the job that needs it.
- In `cd.yml`, `package` and `image-publish` hold `id-token: write` and
  `attestations: write`. Only first-party `actions/*` actions and the
  `docker` CLI may run in those jobs, because any step there could sign
  build provenance. `image-publish` logs out of `ghcr.io` in an `always()`
  step.
- Only `release` has `contents: write`; it runs the third-party
  `softprops/action-gh-release`, which therefore cannot sign provenance.
- Outside CD, only the `codeql` job in `security.yml` writes
  (`security-events: write`).

## Releases

- Releases are cut by the maintainer (`make release VERSION=vX.Y.Z`). Never
  run the `tag`, `push-tag`, or `release` targets, push a tag, or trigger
  CD.
- Archive names (`salus_<os>_<arch>.tar.gz`, `.zip` on Windows),
  `checksums.txt`, provenance attestations, and image tags (`X.Y.Z`, `X.Y`,
  `latest`, `X.Y.Z-<arch>`) are documented in `README.md` and `SECURITY.md`;
  keep the docs and `gh attestation verify` commands in step with `cd.yml`.
- After changing `cd.yml` or an action used only there (artifact actions,
  `actions/attest`, `softprops/action-gh-release`), tell the maintainer to
  run CD manually (`workflow_dispatch`) before the next tag. The
  `Create GitHub Release` and `Publish pre-release` steps and the
  `image-publish` job run only for tags, so recommend a pre-release tag
  (`vX.Y.Z-rc.1`) before a stable one.

## Go version and container image

- CI, CD, and Security install the Go version from `go.mod`
  (`go-version-file`), so the `go` directive decides which standard-library
  fixes ship in release binaries. Keep it at the latest patch release of the
  Go minor version in use.
- The `Dockerfile` builder (`golang:<minor>-alpine<release>`) builds with
  the image's own toolchain, so it must provide at least the `go.mod`
  version. Bump the `go` directive and the builder tag together, and update
  the Go version in `README.md` and `CONTRIBUTING.md`.
- Builder and runtime images are pinned by digest and use the same Alpine
  release; bump their tags together by hand (Dependabot only updates
  digests and patches).
- The runtime stage installs no packages, runs as UID and GID `10001`, and
  creates and chowns `/app/data` before `VOLUME`. `docker.yml` asserts the
  non-root user, volume persistence, and the WARN results for checks whose
  tools the image lacks. Do not add `docker` or `kubectl` to the image or
  document mounting the Docker socket.
- `.dockerignore` limits the build context to what the build needs. Add any
  new non-build directory or local-state file to it.

## Dependencies

- Change `go.mod` and `go.sum` only for an intentional dependency change,
  then run `go mod tidy` and review the diff for unrelated churn.
- Prefer the standard library. A new module needs the maintainer's approval
  and a stated reason.
- Keep `NOTICE` in sync with every module in `go.mod` (version, authors,
  license).
- `.golangci.yml` pins the linter set; change it only when the maintainer
  asks.
