# Contributing to Cryptare

Thanks for your interest in improving Cryptare. This guide covers setup, workflow,
validation, coding expectations and pull-request expectations.

Architecture and maintainability rules live in [`CLAUDE.md`](CLAUDE.md) and
[`.claude/rules/`](.claude/rules/); this guide must stay consistent with them.

## Ground rules

- Create an issue to pitch an addition or change. Pull requests with no corresponding
  issue will be denied.
- If the issue already exists, comment on it before making a pull request to address
  it.
- Confirmation of a pitched concept is required on the issue before making a pull
  request that modifies the code base.
- The license header must be kept on every source file.
- Run `make fmt` (`gofmt -s -w .`) and `make check-all` at the repository root before
  making a pull request.
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Development setup

### Prerequisites

- **Go 1.26 or newer** (`go.mod` declares `go 1.26.0` and selects `toolchain go1.26.9`,
  which an older `go` command downloads automatically unless `GOTOOLCHAIN=local`).
- **A C compiler (gcc or clang) with CGO enabled.** The SQLite driver
  (`github.com/mattn/go-sqlite3`, pulled in by `gorm.io/driver/sqlite`) requires CGO.
  Builds with `CGO_ENABLED=0` compile, but every command that opens the key store (see
  "Build and run") fails at runtime; the other commands still work.
- **Git**, **bash** and **make** (GNU Make; the 3.81 that macOS ships works). On Windows,
  run `make` from Git Bash.
- No linters or scanners to install: `make lint`, `make sec`, `make vuln` and
  `make actionlint` run pinned versions through `go run`: the ones CI uses
  (golangci-lint v2.13.2, gosec v2.29.0, govulncheck v1.8.0) and actionlint v1.7.12,
  which CI doesn't run. `go run` downloads them once and verifies them against the Go
  checksum database. To use a binary you installed instead, override the variable, for
  example `make lint GOLANGCI_LINT=golangci-lint`.
- Optional:
  - shellcheck, which `make actionlint` then uses on the workflows' scripts;
  - Docker, for `make docker-build` and `make docker-smoke`;
  - the dev container in [`.devcontainer/`](.devcontainer/).

### Build and run

```bash
git clone https://github.com/jabbott-iii/Cryptare.git
cd Cryptare
make build                                   # bin/cryptare, stamped with `git describe`
CRYPTARE_DB_PATH="$HOME/.cryptare-dev.db" ./bin/cryptare --help
make help                                    # the targets, grouped
```

The `keys` commands (except `keys path`), the TUI, `encrypt --key` and `decrypt` of a
file encrypted with a stored key open (and create, if missing) the SQLite key store at
`CRYPTARE_DB_PATH`, or at `cryptare/cryptare.db` in your user data folder when the
variable is unset (`./bin/cryptare keys path` prints which). Other commands don't touch
it.
Set `CRYPTARE_DB_PATH` while developing so you don't use your real key store. `*.db`
and `*.ckey` are git-ignored, and CI fails if a key database or key export is tracked.
**Never commit a database file.**

## Workflow

1. Open or find the issue, and get confirmation before starting (see Ground rules).
2. Create a branch from `main`.
3. Read `CLAUDE.md` and the `.claude/rules/` files relevant to your change.
4. Make the smallest complete change. Avoid unrelated refactoring, formatting churn,
   renames and dependency upgrades.
5. Add or update tests (see below).
6. Update documentation:
   - `README.md` for user-visible behaviour, with a line in its "⚠️ Upgrading from …"
     notes when the change affects users of the latest release;
   - `CONTRIBUTING.md`, `CLAUDE.md` and `.claude/` when a command, file, layout or rule
     they describe changes.

   There is no changelog in the repository: describe significant changes in the pull
   request.
7. Run the validation commands, then open a pull request that references the issue.

## Validation

Run these from the repository root. `make check` runs CI's checks
(`.github/workflows/ci.yml`) and a gofmt check that CI doesn't run, and stops at the
first failure:

| Target | What it runs |
|---|---|
| `make fmt-check` | `gofmt -s -l .` must list nothing (`make fmt` fixes it) |
| `make tidy-check` | `go mod tidy -diff`: fails if `go mod tidy` would change `go.mod` or `go.sum` (it changes nothing itself) |
| `make keys-check` | no key database or key export is tracked (SEC-003) |
| `make vet` | `go vet ./...` |
| `make lint` | golangci-lint v2.13.2 |
| `make test-race` | `go test -race -count=1 ./...` (needs CGO; CI runs `-race` on Linux and macOS) |
| `make smoke` | builds `bin/cryptare` and runs CI's smoke test in a temporary folder with its own key store: `--version`, `keys generate`/`keys list`, an encrypt/decrypt round trip, and one with a stored key |

`make check-all` adds `make security` (gosec v2.29.0 and govulncheck v1.8.0, as in
`security.yml`) and `make actionlint`; run it before a pull request. Other targets:
`make test` (quick, cached), `make cover` and `make cover-html` (coverage),
`make golden` (only the golden-fixture tests, for format work),
`make fuzz FUZZ=FuzzExtractTar FUZZTIME=5m` (one fuzz target),
`make docker-build` and `make docker-smoke` (the image, as `docker.yml` tests it), and
`make clean`.

CI runs on Ubuntu, Windows and macOS. After its checks, it builds the binary with
CGO and smoke-tests it: `--version`, `keys generate`/`keys list`, and an encrypt/decrypt
round-trip. On every push and pull request, CodeQL, gosec and govulncheck also run
(`security.yml`), and Dependabot proposes Go module, action and Docker base image updates weekly. Pull requests to `main` get a Docker build smoke test
(`docker.yml`), which also checks that the image runs as UID 10001. Tagged releases get a
signed build provenance attestation for every published file and, after the release, publish
the Docker image for `linux/amd64` and `linux/arm64` to GitHub Packages
(`ghcr.io/<owner>/cryptare`) with its own attestation (`cd.yml`).

## Coding expectations

The full rules are in [`CLAUDE.md`](CLAUDE.md) and [`.claude/rules/`](.claude/rules/).
In short:

- **Layering.** `pkg/crypto.go`, `pkg/format_v2.go`, `pkg/compress.go` and
  `pkg/keys.go` stay UI-agnostic: no printing, prompting, Cobra or Bubble Tea.
- **CLI and TUI stay in step.** Key-store flows go in `pkg/keys.go`, which both use.
  A behaviour change must land in both `logic-cli.go` and `logic-tui.go`, with tests for
  each.
- **Validation that protects data goes in the core layer**, so both interfaces
  inherit it.
- **Compatibility.** Existing encrypted files, directory artifacts, stored keys and
  `.ckey` exports must stay readable. Format changes need a versioned header and a
  legacy read path. The golden fixtures in `pkg/testdata/golden/` must keep
  opening; never regenerate or edit them to make a test pass.
- **Error handling.** Wrap errors with context (`fmt.Errorf("…: %w", err)`) and check
  errors from output writes. Use `closeWithError` for deferred closes on writers.
- **Filesystem.** Write outputs with mode `0o600`. Keep rejecting symlinks, special
  files and path traversal.
- **Dependencies.** Don't add one when the standard library or an existing dependency
  already does the job. Release archives carry the licence of every module linked into
  the binaries: run `scripts/third-party-licenses.sh` after a dependency change, and
  add a module that ships no licence file to its `stated_licence` list once you have
  checked its licence (the release fails until then). An update of `golang.org/x/text`
  that makes `TestNormalizePasswordKnownAnswers` fail changes how passwords are read;
  treat it as a format change (`.claude/rules/formats-and-crypto.md`), not as a test
  to update.
- **Tests.** Use `t.TempDir()`; close database handles (Windows CI depends on it);
  follow the existing CLI (`SetArgs`/`SetIn`/`SetOut`) and TUI (`tea.KeyMsg`) test
  patterns.

## Security

- Never commit secrets, passwords, keys, `.env` files or database files.
- Treat changes to encryption, key storage, password handling or archive extraction as
  security-sensitive. Say so in the pull request and include regression tests.
- Don't weaken or bypass an existing security control (file modes, extraction limits,
  validation, the password policy), and don't disable tests, linters or security scans
  to get a build to pass.
- Don't report suspected vulnerabilities in public issues. Report them privately as
  [`SECURITY.md`](SECURITY.md) describes.
- Keep security fixes at summary level in pull requests, commit messages and docs:
  what changed and how it is tested, not how the weakness could be exploited. Details
  stay in the private advisory.

## Pull request expectations

- It links an issue on which the change was confirmed.
- Its scope is focused on that issue.
- The description says what changed and why, and lists the validation commands you
  ran with their results.
- CI passes on all three operating systems.
- CLI flags and on-disk formats stay backward-compatible, unless the issue explicitly
  approves a breaking change.
- Documentation is updated (see step 6 of the workflow).
- Every change is reviewed by the code owner, @jabbott-iii (see `CODEOWNERS`).
