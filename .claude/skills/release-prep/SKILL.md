---
name: release-prep
description: Prepare Cryptare for a release tag — check docs, versions and validation, and list what the owner must do. Never tags, pushes or publishes.
disable-model-invocation: true
argument-hint: "<version, e.g. v1.4.1>"
---

# Release preparation for $ARGUMENTS

You prepare and report. Tagging and pushing are the owner's
(`make release VERSION=$ARGUMENTS`), and the permission rules deny them.

1. **Version.** `$ARGUMENTS` must pass the Makefile's `check-version` pattern
   (`vMAJOR.MINOR.PATCH`, optionally `-prerelease`; avoid `+build`, which `cd.yml`'s
   image job rejects) and must not exist yet (`git ls-remote --tags origin`).
   Find the previous release: `git describe --tags --abbrev=0` (fetch tags first in a
   shallow clone).

2. **What's in it.** `git log --oneline <previous>..HEAD`. Sort the changes into
   user-visible behaviour, format or compatibility, security, build and release, docs.
   Flag anything that changes an on-disk format or a CLI flag without a matching
   README upgrade note.

3. **README.**
   - Upgrade notes: every user-visible change since the previous tag is covered. Check
     the "(changes on `main` since vX)" qualifiers and propose wording for the owner
     now that these changes ship in `$ARGUMENTS`.
   - Version-dependent statements ("Releases after v…", example tags such as
     `download/v…` and image tags like `1.4.0`) still read correctly.
   - "Release archives" table matches the `cd.yml` matrix.

4. **SECURITY.md.** If `$ARGUMENTS` starts a new minor line, the supported-versions
   table moves to it. Also check it already matches the latest released line.

5. **Golden fixtures.** If the golden `README.md` calls fixtures "unreleased", note
   whether this release makes them released; propose the wording, don't change the
   fixtures.

6. **Consistency.** `bash .claude/skills/doc-audit/check-versions.sh` and
   `bash .claude/skills/doc-audit/check-links.sh`.

7. **Validation.** `make check-all`; `scripts/third-party-licenses.sh` (fails on a
   module with no recorded licence); `make docker-smoke` if Docker is available.
   Report what ran and what couldn't.

8. **Report**: a draft of release notes (summary-level for security fixes), the doc
   edits you made or propose, validation results, and the owner's next steps:
   review, commit, `make release VERSION=$ARGUMENTS`, then watch `cd.yml`.
