---
paths:
  - "pkg/format_v2.go"
  - "pkg/format_v2_test.go"
  - "pkg/crypto.go"
  - "pkg/crypto_test.go"
  - "pkg/keys.go"
  - "pkg/keys_test.go"
  - "pkg/golden_test.go"
  - "pkg/legacy_fixtures_test.go"
  - "pkg/password_norm_test.go"
  - "pkg/fuzz_test.go"
  - "pkg/testdata/**"
---

# On-disk formats, key derivation and passwords

These files define what Cryptare writes and how it reads everything earlier releases
wrote. Run `/format-change` before changing anything here that affects bytes on disk,
key derivation or how a password becomes key material.

## Compatibility contract

- New data is written in the version 2 format: magic `"CRYPTARE\x00"`, a 46-byte header
  (54 with a stored key's ID), and AES-256-GCM chunks whose additional data is the whole
  header. The authoritative layout is the header comment and constants in
  `format_v2.go`; keep that comment accurate when the code changes.
- Version 1 (legacy, PBKDF2, no header) is read-only and stays readable with no time
  limit. There is no migration command. `legacy_fixtures_test.go` holds the test-only
  legacy writers.
- Any change to what is written needs a new format version, content type, key source
  or KDF identifier, a read path for everything already written, and tests for both.
  Choosing a new identifier is the owner's decision; propose, don't decide.
- Readers refuse unknown versions, IDs and unsafe parameters before deriving a key
  (`errUnsupportedFormat`; Argon2id read limits 1 GiB, 1–10 passes, 1–16 lanes; chunk
  size 2^10–2^24). Don't relax these.
- Wrong password or tampering fails closed with the generic `errDecrypt`. The first
  chunk is authenticated before any output exists; output is released only after the
  last chunk authenticates.

## Passwords and keys

- `CheckPasswordPolicy` (`MinPasswordLength` = 15 code points, not one repeated
  character) runs on every path that protects new data: `EncryptFile`,
  `EncryptKeyBlob`, `ExportKeyToFile`, and `StoredKeyCredential` when unlocking for
  encryption. Read paths (decrypt, `DecryptKeyBlob`, import) must not apply it.
- `normalizePassword` (NFKC, leading BOM dropped) is part of the format: KDF 2 data
  opens only through the normalised form. `TestNormalizePasswordKnownAnswers` pins it;
  if it fails, stop and report — don't update the expected values.
- `passwordCandidates` tries at most three forms for KDF 1 and legacy data. Keep that
  bound.
- A stored key encrypts files through a per-file HKDF-SHA256 key, and the header names
  the key (key source 2, KDF 3). Stored keys and key exports are always protected by a
  password. An export is protected by the key's own master password.

## Guard tests

Keep these passing without changing their expectations:

- golden: `TestGolden*` (`golden_test.go`, `testdata/golden/`; `make golden`);
- v2: `TestEncryptWritesVersion2Format`, `TestDefaultPasswordKDFIsWritten`,
  `TestStreamRoundTripSizes`, `TestStreamRejectsTampering`,
  `TestDecryptRejectsUnsupportedHeaders`, `TestDecryptRejectsWrongContentType`;
- stored keys: `TestStoredKeyEncryptDecrypt`, `TestStoredKeyHeaderIsAuthenticated`,
  `TestStoredKeyHeaderChecks`;
- legacy: `TestDecryptLegacyEncryptedFile`, `TestDecryptAcceptsLegacyEmptyPassword`,
  `TestDecryptAcceptsLegacyShortPassword`, `TestLegacyShortPasswordCmds`, `TestDeriveKey`;
- passwords: `TestNormalizePasswordKnownAnswers`, `TestNormalizedPasswordHeader`,
  `TestEarlierDataOpensWithPasswordForms`, `TestUnicodePasswordFormsOpenTheSameData`.

Fuzz targets for untrusted input: `FuzzParseV2Header`, `FuzzDecryptingReader`,
`FuzzKeyExportValidation` (`make fuzz FUZZ=<name> FUZZTIME=1m`).

## Golden fixtures

`pkg/testdata/golden/` holds artifacts written by released binaries, with their
passwords listed in its `README.md`. Never edit, regenerate, rename or delete a fixture
or its `.gitattributes`; a PreToolUse hook blocks edits to tracked fixtures. Adding a
fixture for a new format version is allowed: write it with a real build, keep key
material as `.txt` (the `.ckey`/`.db` names are refused by `make keys-check`), and add
its row to the README table.
