# Golden format fixtures

Artifacts written by released Cryptare binaries (and two by the development build of
v1.3.2; see the table), which `golden_test.go` must keep opening. They pin the on-disk
formats: a change that broke the writer and the reader in the same way would still pass
the round-trip tests, but not these. Never regenerate or edit a fixture to make a test
pass; a failure here means existing users' files would stop opening.

They are test vectors, not secrets: every password is written below, and the stored keys
protect nothing else. Key exports and stored keys are kept as `.txt` (their base64 text)
rather than `.ckey` and `.db`, the names that CI's SEC-003 guard and `make keys-check`
refuse for real key material.

## Passwords

| Name | Value |
|---|---|
| `P` | `golden-fixture-password-1` |
| `P2` | `golden-fixture-export-only-2` |
| `PN` | `golden ﬁxture ｐａｓｓｗｏｒｄ` (U+FB01 ligature and full-width letters; its NFKC form is `golden fixture password`) |

## Plaintext

`plain/hello.txt` is the file, and `plain/tree/` (`a.txt`, `sub/b.txt`) the folder,
that the fixtures encrypt.

## Fixtures

| File | Written by | How |
|---|---|---|
| `v1-file.enc` | v1.0.1 | `encrypt hello.txt -p P`: legacy file, PBKDF2 |
| `v1-folder.enc` | v1.0.1 | `encrypt tree -p P`: legacy folder (`CRYPTARE-DIR-ENC`) |
| `v1-stored-key.txt` | v1.0.1 | `keys generate -p P`, the `encrypted_blob` of key `2e5fe1f18a8a72d2` |
| `v1-key-export.txt` | v1.0.1 | `keys export 2e5fe1f18a8a72d2 -p P` |
| `v2-file-kdf1.enc` | v1.3.1 | `encrypt hello.txt --password P`: version 2, KDF 1 |
| `v2-file-kdf2.enc` | v1.3.1 | `encrypt hello.txt --password-file` with `PN`: version 2, KDF 2 (NFKC) |
| `v2-folder.enc` | v1.3.1 | `encrypt tree --password P` |
| `v2-stored-key.txt` | v1.3.1 | `keys generate --password P`, the `encrypted_blob` of key `b49800c1a2913f46` |
| `v2-key-export.txt` | v1.3.1 | `keys export b49800c1a2913f46 --password P` |
| `v2-key-export-separate-password.txt` | v1.3.1 | `keys export b49800c1a2913f46 --password P2`: an export with a password of its own (BUG-011) |
| `v2-storedkey-file.enc` | development build of v1.3.2, the first with stored keys (plan 3.4) | `keys import v2-key-export.txt`, then `encrypt hello.txt --key b49800c1a2913f46`: key source 2 |
| `v2-storedkey-folder.enc` | same | `encrypt tree --key b49800c1a2913f46` |

The released binaries were the `cryptare_linux_amd64` assets of the GitHub releases,
checked against each release's `checksums.txt`:

- v1.0.1: `cryptare_linux_amd64.tar.gz`, SHA-256 `f38c2f756c75d61742bf1b06c015f7ba4b3d3b810531b295c24c5573663a013a`
- v1.3.1: `cryptare_linux_amd64.tar.gz`, SHA-256 `37f1945ed275cc46d87415e47f97029ab7461ac07a1842cdb7197b6ca8859614`

Generated on 2026-10-04. The stored-key fixtures pin the key source 2 layout from the
development build that introduced it. v1.3.2 released that code, and the writer is
unchanged since, so they are what v1.3.2 and later write.
