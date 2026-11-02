---
name: release-prep
description: Checks that Capsus is ready to release a given version, prepares the documentation that changes with a release, and prints the release command for the maintainer to run. Never tags or pushes.
argument-hint: "vX.Y.Z[-prerelease]"
disable-model-invocation: true
allowed-tools: Bash(git status *) Bash(git branch *) Bash(git log *) Bash(git for-each-ref *) Bash(git ls-remote *) Bash(git fetch --tags *) Bash(git rev-parse *) Bash(make deps-drift)
---

# Release preparation for `$ARGUMENTS`

You prepare; the maintainer releases. Never run `make release`, `make tag`, `make push-tag`,
`git tag`, `git push` or `gh release`, and never a tool's release target. Leave any doc edits
uncommitted.

## 1. Version

- `$ARGUMENTS` must match `^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`, the pattern the
  `Makefile`'s `check-version` enforces. It is a Capsus version: the tag starts `cd.yml` for
  Capsus and has nothing to do with the bundled tools' versions.
- `git fetch --tags origin` (it only fetches), then confirm the tag exists neither locally
  (`git rev-parse -q --verify refs/tags/<tag>`) nor on `origin` (`git ls-remote --tags origin`),
  and that it is greater than the latest release
  (`git for-each-ref --sort=-v:refname --format='%(refname:short)' refs/tags`).
- The first release is `v0.1.0` (P-050).

## 2. Readiness

- On `main`, up to date with `origin/main`, with a clean working tree. Otherwise report it and
  stop.
- Plan 1 Phases 1 to 4 are done: the tools imported at release tags, the launcher and its tests,
  the `Dockerfile` `static` stage, `THIRD_PARTY_LICENSES` and the combined `NOTICE.txt`
  (P-040, P-041), and README features verified against a built binary (P-042).
- `go run ./tools/licenses -check` passes: `THIRD_PARTY_LICENSES` matches the modules the
  release binaries compile in and `NOTICE.txt` lists them. `cd.yml` packages the committed file
  without checking it.
- The bundled versions in the registry match the imported tags (P-011), and `make deps-drift`
  has been reviewed.
- The `Dockerfile`'s builder carries the latest Go 1.26 patch release (D-9): compare
  `GOLANG_VERSION` in `docker buildx imagetools inspect <image>@<digest> --format '{{json .Image}}'`
  with <https://go.dev/dl/>. If it is older, ask the maintainer to merge Dependabot's docker update
  first.
- Ask the maintainer to confirm that CI, Security and Docker are green on that commit; you can't
  see GitHub runs unless `gh run list` works in this session.
- SEC-001: ask whether the GitHub tag ruleset for `v*` tags exists, and report the answer.
- If the maintainer wants a local re-check, run `/pre-pr <previous release tag>`.

## 3. Documentation

Run `/docs-drift all` and resolve every **wrong** finding before the tag. For a new minor or
major release line, propose the `SECURITY.md` supported-versions table (and a reporting process
while it is still GitHub's template); apply it only after the maintainer confirms.

## 4. Hand-off

Print, for the maintainer to run:

```bash
make release VERSION=$ARGUMENTS
```

Then list the post-release checks:

- The CD run is green: `build` (all six targets), `package` and `release`.
- The GitHub Release has the six archives (`.tar.gz` for Linux and macOS, `.zip` for Windows)
  and `checksums.txt`; each archive holds the binary, `LICENSE`, `NOTICE.txt` and
  `THIRD_PARTY_LICENSES`.
- `gh attestation verify <archive> --repo jabbott-iii/Capsus` passes for one archive.
- There is no container image to check (D-8).
