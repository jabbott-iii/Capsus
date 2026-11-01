---
paths:
  - "**/*.md"
---

# Writing and maintaining documentation

- **Ground every statement.** Each command, flag, default, limit, path, version,
  platform, asset name and message must match the code, Makefile, workflows or
  Dockerfile. Find it before you write it. Don't claim a command's output without
  running it. If something can't be verified, leave it out or mark it `TODO` for the
  owner.
- **Edit in place.** Change only the sections affected. Preserve accurate content, the
  existing order and headings, and the voice: British spelling (behaviour, licence,
  normalise), short plain sentences, `code` for commands, flags, paths and identifiers,
  copyable commands run from the repository root.
- **README order** (keep it): demo images, title and description, Features, Use cases,
  Prerequisites, Installation, Quick start (with the safety and upgrade notes),
  Core CLI capabilities, Configuration, Docker, Testing and quality checks, Project
  structure, Contributing and license.
- **Upgrade notes.** A change that alters behaviour for users of the latest release
  goes under README's "⚠️ Upgrading from …" notes, as one bullet that says what changed
  and what the user must do.
- **CONTRIBUTING** covers setup, workflow, validation, coding and PR expectations. It
  must agree with `CLAUDE.md`, `.claude/rules/` and the Makefile.
- **Links** are relative within the repository and must resolve. `/doc-audit` checks
  them.
- **Security text** is summary-level only: what changed and how it is tested. No
  exploit detail, reproduction steps, proof-of-concept input or pointers to commits
  that hold key material.
- **No new badges, screenshots, diagrams or external links** unless they already exist
  or materially help. The README's demo images (above the title) are GitHub
  user-attachment uploads; the recordings are scripted in `demo/*.tape` (VHS).
- **Instruction files** (`CLAUDE.md`, `.claude/rules/*.md`, `.claude/skills/*/SKILL.md`,
  `.claude/agents/*.md`): keep them short and specific to this repository, and update
  them when a command, file, layout or rule they describe changes. When a rule file's
  subject files move, update its `paths:` list.
