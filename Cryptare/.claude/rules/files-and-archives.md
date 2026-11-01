---
paths:
  - "pkg/compress.go"
  - "pkg/compress_test.go"
  - "pkg/cancel_test.go"
  - "pkg/interrupt_test.go"
  - "pkg/interrupt_unix_test.go"
---

# Filesystem safety, compression and extraction

Archive extraction and file writes are security-sensitive. Keep every check below;
new code that writes, extracts or decompresses follows the same rules.

- **Atomic single-file writes** go through `writeFileAtomic` or `createAtomicFile` +
  `Commit`: a hidden temporary file next to the destination, renamed on success. A
  failed or cancelled run leaves the destination as it was.
- **Atomic extraction.** `extractToDir` extracts into a new hidden `.<name>.*.tmp`
  folder (0700) and renames it into place on success. With `--force` an existing
  output is replaced as a whole, never merged into. Replacing a folder that holds the
  archive is refused (`ErrInputInsideOutput`).
- **Confinement.** Entries are written through an `os.Root` on the new folder (the
  exception is a zip holding a single file extracted to a file, which goes through
  `createAtomicFile`). Path traversal (`..`) is rejected before that. Archive permissions are ignored: folders
  0700, files 0600 (0700 when marked executable).
- **Limits.** Every decompression or extraction runs under `ExtractLimits`
  (`DefaultExtractLimits`: 10 GiB, 100,000 entries) and copies through `extractBudget`,
  never a bare `io.Copy` from a decompressor. Input read whole is capped too
  (`readAllLimit`).
- **Reading trees.** Source folders are always walked with `walkSourceTree` (an
  `os.Root` on the source). Symlinks and special files are rejected.
- **Output safety.** Core refuses an output that is its own input
  (`ErrSameInputOutput`) and a folder's output inside that folder
  (`ErrOutputInsideInput`). Containment uses `pathWithin` (text, then folder identity
  via `os.SameFile`), not string prefixes.
- **Names.** Temporary names use `tempNamePart` (at most 64 bytes of the output's
  name). Gzip header names go through `gzipHeaderName` (Latin-1 only). Output names
  never come from a stored gzip name, but a stored name ending in `.tar` makes
  `decompress` treat the data as a tarball (`isTarGzArchive`).
- **Compression options.** `resolveCompressFormat` refuses a `--format` that
  contradicts the output extension (`ErrFormatMismatch`).
- **Cancellation tests** use `cancelAfter` (a context that cancels after a set number
  of checks), so their outcome doesn't depend on timing. Signal tests run the binary as a subprocess through the
  `TestRunMain` helper.
- Fuzz targets: `FuzzExtractTar`, `FuzzExtractZip`, `FuzzParseSize`. The extraction
  fuzzers need a short minimisation time, which `make fuzz` sets
  (`-fuzzminimizetime 3s`); without it they look hung.
- Run memory-heavy probes (decompression bombs) under a memory-capped cgroup such as
  `systemd-run --user --scope -p MemoryMax=768M -p MemorySwapMax=0 <command>`, not
  `ulimit -v`.
