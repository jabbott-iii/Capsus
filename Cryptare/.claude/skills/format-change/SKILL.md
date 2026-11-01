---
name: format-change
description: Checklist for any change that affects what Cryptare writes to or reads from disk — v2 header fields or IDs, Argon2id/HKDF parameters, chunking, legacy readers, stored-key blobs, key exports, password normalisation — or a golang.org/x/text or Go update that breaks TestNormalizePasswordKnownAnswers. Use before writing the change.
---

# Changing an on-disk format or key derivation

Existing users' files, stored keys and exports must keep opening. Read
`.claude/rules/formats-and-crypto.md` and the header comment in `pkg/format_v2.go`
first.

1. **Classify the change.**
   - *Reader only* (accept more, refuse something unsafe): allowed if every artifact a
     release wrote still opens. Prove it with the golden tests.
   - *Writer changes bytes on disk*: needs a new format version, content type, key
     source or KDF identifier, so older data keeps its meaning.
   - *Normalisation or Unicode tables change* (`x/text`, Go minor release): the
     normalised form of some passwords changes, and KDF 2 data protected with them
     would stop opening.

2. **Stop for the owner** before choosing a new identifier, dropping a read path, or
   accepting a normalisation change. Present: what changes, which existing data is
   affected, how it stays readable (extra read candidate, new KDF ID, or a documented
   break), and what older releases do with the new data ("unsupported encrypted data:
   …"). Earlier decisions of this kind are in `git show 40c3464:intel/notes.md` (§4)
   and `…:intel/maint.md` (§3).

3. **Implement** with the existing pieces: header parse/marshal and their limits in
   `format_v2.go`, `passwordCandidates` (at most three derivations), legacy readers in
   `crypto.go`, `validateKeyExport`/`validStoredKeyBlob` for exports. Keep checks that
   refuse unsafe headers before key derivation.

4. **Tests.**
   - Round trip for the new form, and refusal of malformed or mismatched headers.
   - Every existing guard test still passes unchanged (list in the rule file).
   - `make golden` passes without touching any fixture.
   - New format version: add fixtures written by a real build (named like the existing
     ones, key material as `.txt`) and their rows in `pkg/testdata/golden/README.md`,
     with the commands and passwords used.
   - Extend the fuzz seed corpora if the header grows; run `FuzzParseV2Header`,
     `FuzzDecryptingReader` and `FuzzKeyExportValidation` for at least 1 minute each.

5. **Docs** (`/doc-sync`): the `format_v2.go` header comment; README "File format"
   paragraph; an upgrade note saying which releases can't read the new data and what
   users do; the golden README.

6. **Review.** Ask the `change-reviewer` agent to review the diff for compatibility
   before you report the change done.

7. **Report** the classification, the owner decision it relied on, tests added, and
   the exact validation run with results.
