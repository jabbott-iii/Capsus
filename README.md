<img width="1200" height="700" alt="cryptare-cli" src="https://github.com/user-attachments/assets/95104d1a-5feb-4b75-b6bd-8389136f87ba" />
<img width="1200" height="640" alt="cryptare-tui" src="https://github.com/user-attachments/assets/59d9167c-66e0-42ef-aa62-84a6bc37d465" />

# Cryptare

Cryptare is a terminal tool for encrypting, decrypting, compressing and extracting files and directories. It combines a scriptable command-line interface with an interactive terminal UI and a small local store for password-protected keys. It is meant for people who want password-based file protection and archiving from the command line.

## Features

- **File Encryption & Decryption**
  - Encrypt files of any size with AES-256-GCM, streamed in authenticated chunks, under a key derived from your password with Argon2id
  - Encrypt directories into a single encrypted archive
  - Decrypt previously encrypted files
  - Decrypt encrypted directory archives back into their original tree
  - Optional custom output paths
  - Password-based protection for secure workflows

- **Compression & Decompression**
  - Compress files with gzip
  - Compress files or directories as zip archives
  - Compress directories as tar.gz archives
  - Decompress gzip files, tar.gz archives, and zip archives
  - Optional compression levels
  - Automatic output name derivation

- **Key Management**
  - Generate and store random encryption keys, each protected by a master password
  - Encrypt files and directories with a stored key instead of a password; decrypting finds the key from the file
  - List stored keys
  - Export keys to encrypted files, protected by the key's own master password, to back them up or move them
  - Import keys from encrypted files
  - Delete stored keys with confirmation safeguards

- **Interactive TUI**
  - Terminal user interface for file and key management
  - Launches automatically when run without subcommands

## Use cases

- Protect a sensitive document before copying it to a USB drive or cloud storage: `cryptare encrypt ./tax-return.pdf`
- Back up a project folder as one encrypted file, then restore it later: `cryptare encrypt ./project-dir --output ./project-dir-backup.enc`, then `cryptare decrypt ./project-dir-backup.enc --output ./restored-project-dir`
- Encrypt with a stored key instead of choosing a password for each file: `cryptare keys generate`, then `cryptare encrypt ./report.pdf --key <key-id>`; `cryptare decrypt ./report.pdf.enc --output ./report-copy.pdf` finds the key and asks for its master password. Keep an export of the key (`cryptare keys export <key-id>`) somewhere safe: files encrypted with it can't be decrypted without it
- Package build output as a tar.gz or zip archive for sharing, and extract archives you receive: `cryptare compress ./build --format zip`, `cryptare decompress ./build-backup.tar.gz`
- Use menus instead of flags: run `cryptare` (or `cryptare --vim` for vim-style keys)

## Prerequisites

- To build from source: Go 1.26 or newer (`go.mod` selects Go 1.26.9, which an older `go` command downloads automatically unless `GOTOOLCHAIN=local` is set), plus a C compiler (gcc or clang) with CGO enabled. The SQLite driver used for the key store requires CGO. On Windows this means a CGO-capable toolchain such as MinGW-w64.
- Optional: Docker, to build and run the container image.

## Installation

### Build from source

```bash
git clone https://github.com/jabbott-iii/Cryptare.git
cd Cryptare
CGO_ENABLED=1 go build -o cryptare .
sudo mv cryptare /usr/local/bin/cryptare   # optional: put it on your PATH
```

### Release archives

Tagged releases publish these assets, built by `.github/workflows/cd.yml`, together with a `checksums.txt` file (SHA-256):

| Platform | Asset |
|---|---|
| Linux x86-64 | `cryptare_linux_amd64.tar.gz` |
| Linux ARM64 | `cryptare_linux_arm64.tar.gz` |
| macOS Intel | `cryptare_darwin_amd64.tar.gz` |
| macOS Apple silicon | `cryptare_darwin_arm64.tar.gz` |
| Windows x86-64 | `cryptare_windows_amd64.zip` |

Each archive holds the binary named after it (`cryptare_linux_amd64`, `cryptare_windows_amd64.exe`, …). Archives of releases after v1.2.0 also hold `LICENSE`, `NOTICE` and `THIRD_PARTY_LICENSES.txt`, which has the licences of the Go standard library and the Go modules built into the binary and, in releases after v1.3.0, of the C libraries linked into it (musl on Linux, the MinGW-w64 runtime on Windows). The Linux binaries are statically linked, so they run on any distribution; releases after v1.3.0 link them against musl instead of glibc. There is no Windows ARM64 build.

> ⚠️ The v1.0.0 binaries were built with CGO disabled and exit on every command with `go-sqlite3 requires cgo to work`. Use v1.0.1 or later (BUG-001).

To install the latest release on Linux, download its archive and `checksums.txt` from the [Releases page](https://github.com/jabbott-iii/Cryptare/releases), check the hash, and put the binary on your PATH (for a specific release, replace `latest/download` with `download/<tag>`, such as `download/v1.4.0`). The archive has no top-level folder, so the commands below start in a new, empty folder:

```bash
cd "$(mktemp -d)"
BASE=https://github.com/jabbott-iii/Cryptare/releases/latest/download
curl -LO "$BASE/cryptare_linux_amd64.tar.gz"
curl -LO "$BASE/checksums.txt"
sha256sum --ignore-missing -c checksums.txt
tar -xzf cryptare_linux_amd64.tar.gz
chmod +x cryptare_linux_amd64
sudo mv cryptare_linux_amd64 /usr/local/bin/cryptare
```

- macOS: use the matching `darwin` archive in the same way, and check its hash with `shasum -a 256`.
- Windows: extract the zip, rename `cryptare_windows_amd64.exe` to `cryptare.exe`, and add it to your PATH.

`checksums.txt` only shows that a download wasn't damaged. Releases after v1.3.1 also carry a signed build provenance attestation for every file, which shows it was built by this repository's release workflow. Check one with the [GitHub CLI](https://cli.github.com/):

```bash
gh attestation verify cryptare_linux_amd64.tar.gz --repo jabbott-iii/Cryptare
```

## Quick start

```bash
cryptare encrypt ./secret.txt                                # prompts for a password; prints "Encrypted: ./secret.txt → ./secret.txt.enc"
cryptare decrypt ./secret.txt.enc --output ./restored.txt    # prompts for the password; prints "Decrypted: ./secret.txt.enc → ./restored.txt"
cryptare                                                     # opens the interactive TUI
```

- Commands don't overwrite anything by default:
  - if the output file or folder already exists, the command stops; add `--force` to overwrite it;
  - `encrypt` leaves the original in place, so `decrypt` without `--output` stops while the original file is still there;
  - an output that is the input itself is always refused, even with `--force`, as is compressing or encrypting a folder into a file inside that folder;
  - `keys export` follows the same rule (`--force` to overwrite), and never writes over the key store itself;
  - the TUI never overwrites, so choose a different output path there;
  - a command that fails part-way, or that you stop, leaves no partial output, and a file or folder it was replacing with `--force` is kept;
  - with `--force`, an existing output folder is replaced as a whole, not merged into: files in it that aren't in the archive are removed. Extracting into the folder that holds the archive itself is refused;
  - on Linux and macOS, everything the tool writes is private to you: output files are mode 0600, and extracted or decrypted folders are 0700 with files inside at 0600 (0700 for files the archive marks executable). Permissions stored in an archive are otherwise ignored. **On Windows** these modes don't apply: what Cryptare writes, the key store included, gets the permissions of the folder it is written to. Your own profile folders are normally private, but a shared location such as `C:\Users\Public` or a shared drive is readable by everyone who can read that folder, so write decrypted output and the key store to a folder only you can read.
- Stopping a command: Ctrl+C (or `kill`, or closing the terminal) during `encrypt`, `decrypt`, `compress` or `decompress` stops it and removes its unfinished output. It exits with status 128 plus the signal number (130 for Ctrl+C), and a second Ctrl+C ends it at once. At the hidden password prompt, any of these signals ends the command with your terminal's echo restored. Quitting the TUI while an action runs cancels the action first ("Cancelling…"). A signal the command doesn't catch, such as `kill -9`, Ctrl+\ (`SIGQUIT`) or that second Ctrl+C, or a power loss can still leave a hidden `.<name>.*.tmp` file or folder behind.
- Extraction is limited to protect against decompression bombs: `decompress`, and `decrypt` for an encrypted folder, stop after 10 GiB of output or 100,000 archive entries, and remove what they wrote. Change the limits with `--max-size` and `--max-entries` (`0` means no limit). The TUI always uses the defaults.
- The password prompt reads the whole line, spaces included, and hides what you type when run in a terminal. When input is piped in, the first line is used.
- New passwords must be at least 15 characters long, and a single repeated character (such as `aaaaaaaaaaaaaaa`) is refused. Any characters count, including spaces, and no mix of character types is required, so a few unrelated words make a good password. This applies to `encrypt` and `keys generate`, whether the password comes from the prompt, `--password`, `--password-file` or the TUI. `encrypt --key` and `keys export` use a key's existing master password, which must meet the rule too, so a key stored by an earlier version under a shorter password still decrypts but can't encrypt new files or be exported.
- When you type a new password in a terminal or the TUI, you're asked to type it again to confirm it. Piped input is read once.
- A password is the same however its characters were entered: an accented letter typed as one character or as a letter followed by a combining accent, full-width letters, ligatures and similar variants are made equal (Unicode NFKC normalisation), and a byte order mark that an editor put at the start of a `--password-file` is ignored. The 15-character minimum counts the password after this.
- Decrypting and importing accept any password, so files and keys protected with a shorter password before this rule existed still open.
- In scripts, keep passwords off the command line: use `--password-file` (the first line of a file, ideally one only you can read) or pipe the password in (`cryptare encrypt ./secret.txt < pw.txt`). `--password` still works but prints a warning, because other users can see command-line arguments and your shell saves them in its history.
- ⚠️ **Upgrading from v1.0.1 or earlier:** the old prompt kept only the text before the first space. If you encrypted a file at the prompt with a multi-word passphrase, decrypt it with just the first word (SEC-002).
- ⚠️ **Upgrading from v1.1.0 or earlier:**
  - new encrypted files, folders, stored keys and key exports use the versioned format (see "File format" below), which v1.1.0 and earlier can't read. Upgrade every machine that needs to decrypt them; this version still reads everything older versions wrote, apart from the rare v1.3.0 case below;
  - scripts that pass `encrypt`, `keys generate` or `keys export` a password shorter than 15 characters now fail. (Releases up to v1.3.1 let you export a key whose master password is shorter under a separate, longer export password; later versions protect an export with the key's own master password, so such a key can't be exported. See the v1.3.1 note below);
  - `decompress --force` and `decrypt --force` now replace an existing output folder instead of adding to it;
  - archives over 10 GiB of output or 100,000 entries need `--max-size` or `--max-entries`;
  - `--password` now prints a warning on stderr; switch scripts to `--password-file` or piped input;
  - on Linux and macOS, the commands that open the key store refuse one that another user owns or that others can write to (see `CRYPTARE_DB_PATH` under [Configuration](#configuration));
  - extracted files and folders no longer keep the archive's permissions; they are owner-only (see above);
  - `keys export` refuses an existing output file unless you add `--force`, and always refuses the key store;
  - `encrypt dir/` and `encrypt .` now write the encrypted file next to the folder (`dir.enc`, `<parent>/<name>.enc`) instead of inside it, and an `--output` inside the folder being encrypted is refused;
  - `compress --format` that contradicts the `--output` extension, and `--level` values other than 1–9 or -1, are refused instead of being silently replaced (the TUI checks the same);
  - `--password ""` now means an empty password instead of prompting;
  - files without Cryptare's format header (from v1.1.0 and earlier) are read whole, so `decrypt` refuses one larger than `--max-size` (default 10 GiB) before reading it; raise `--max-size` for a bigger one. `keys import` refuses files over 1 MiB;
  - a `CRYPTARE_DB_PATH` containing `?` is refused (SQLite opened a different file); use a `file:` URI with `%3F`;
  - the Docker image runs as an unprivileged user (UID 10001); for a bind-mounted data folder, add `--user "$(id -u):$(id -g)"` (see [Docker](#docker)).
- ⚠️ **Upgrading from v1.2.0 or earlier:**
  - the key store is no longer `cryptare.db` in the current folder but `cryptare/cryptare.db` in your user data folder (see `CRYPTARE_DB_PATH` under [Configuration](#configuration)); `cryptare keys path` prints where it is. When `CRYPTARE_DB_PATH` isn't set and a command that uses the key store, or `keys path`, finds a `cryptare.db` in the current folder, it says so on stderr and shows how to keep using it with `CRYPTARE_DB_PATH` or, while there is no key store in the new location yet, how to move it there. It never opens or moves that file itself;
  - new data protected with a password that normalisation changes (accents typed as combining characters, full-width letters, a `--password-file` starting with a byte order mark, …) records this in its header, and v1.2.0 and earlier refuse it with "unsupported encrypted data: key derivation 2". Data protected with any other password is written as before, and v1.2.0 reads it;
  - data that v1.2.0 or earlier protected with a password typed with combining accents still needs the password typed that way.
- ⚠️ **Upgrading from v1.3.0:** v1.3.0 normalised a rare kind of password incorrectly: one with a vowel sign or length mark from scripts such as Tamil, Malayalam, Bengali, Oriya, Kannada, Sinhala or Myanmar, followed later by a combining accent such as an acute. A file that v1.3.0 protected with such a password doesn't open in later versions. Decrypt it with v1.3.0 and encrypt it again. Every other password works as before.
- ⚠️ **Upgrading from v1.3.1 or earlier:**
  - stored keys can encrypt files (`encrypt --key <key-id>`, or the "Stored key ID" field in the TUI). A file encrypted with a stored key names the key in its header, and v1.3.1 and earlier refuse it with "unsupported encrypted data: key source 2";
  - `keys export` asks once for the key's master password, checks it against the key, and protects the export with it, so a key has one password everywhere. `--password` and `--password-file` give that master password: a script that passed a different export password now fails with "wrong master password", and a key whose master password is shorter than 15 characters can't be exported (generate a new key instead);
  - `keys import` checks that the key inside the export opens. An export made by v1.3.1 or earlier with a separate export password needs both passwords, in either order: give the second with `--export-password-file` (or at the prompt, at a terminal), or in the TUI's second password field;
  - `encrypt --key`, and `decrypt` of a file encrypted with a stored key, open the key store; other file commands still never do;
  - a command that fails no longer prints its usage text after the error; only mistakes in the command line itself (an unknown flag, a missing argument) do.

## Core CLI capabilities

Cryptare is organised into focused command groups:

- cryptare encrypt — encrypt a file or directory
- cryptare decrypt — decrypt a file or encrypted directory archive
- cryptare compress — compress a file or directory
- cryptare decompress — decompress a gzip file or extract a tar.gz/zip archive
- cryptare keys — manage encryption keys
- cryptare --version (or -v) — print the version: release builds report their tag, `make build` reports what `git describe` prints, and a plain `go build` reports `dev`

Short forms: `-o` for `--output`, `-p` for `--password`, `-k` for `--key`, `-f` and `-l` for `compress --format` and `--level`, and `-y` for `keys delete --yes`.

### encrypt

- cryptare encrypt [path] — encrypt a file with AES-256-GCM or package a directory into a single encrypted archive
- cryptare encrypt [path] --output [path] — write to a custom output file
- cryptare encrypt [path] --force — overwrite the output if it already exists
- cryptare encrypt [path] --password-file [file] — read the encryption password from the first line of a file
- cryptare encrypt [path] --password [value] — provide the password on the command line (prints a warning; prefer --password-file)
- cryptare encrypt [path] --key [key-id] — encrypt with a stored key instead of a password (`-k` for short); it asks for the key's master password, which `--password-file` can give instead

Examples:
- cryptare encrypt ./secret.txt
- cryptare encrypt ./secret.txt --output ./secret.txt.enc
- cryptare encrypt ./project-dir --output ./project-dir-backup.enc
- cryptare encrypt ./secret.txt --password-file ~/.config/cryptare/pw.txt
- cryptare encrypt ./secret.txt --key 3f9a1c0e7b2d4a6f

File format: encrypted files and folders, stored keys and key exports are written in Cryptare's versioned format. A file encrypted with a stored key gets a key of its own, derived from the stored key, and its header names the stored key; versions up to v1.3.1 can't read it. Each password use runs Argon2id with 64 MiB of memory, 3 passes and 4 lanes, so it takes a moment. Files are encrypted and decrypted as a stream, so memory use doesn't grow with file size. This version still reads files, keys and exports made by earlier versions (apart from the rare v1.3.0 case in the upgrade notes), and there's nothing to convert. Versions up to v1.1.0 can't read the versioned format: they report it as a wrong password or corrupted file. The layout is described in the header comment of [`pkg/format_v2.go`](pkg/format_v2.go).

### decrypt

- cryptare decrypt [path] — decrypt an AES-256-GCM encrypted file or restore an encrypted directory archive
- cryptare decrypt [path] --output [path] — write to a custom output file or restore into a target directory
- cryptare decrypt [path] --force — overwrite the output if it already exists
- cryptare decrypt [path] --password-file [file] — read the decryption password from the first line of a file
- cryptare decrypt [path] --password [value] — provide the password on the command line (prints a warning; prefer --password-file)
- a file encrypted with a stored key names the key in its header: `decrypt` takes the key from the key store and asks for its master password (or reads it from `--password-file`). The key has to be in the key store; import its export with `keys import` first if it isn't. The header is read from regular files only, so decrypt such a file from a file, not a pipe
- cryptare decrypt [path] --max-size [size] --max-entries [n] — change the extraction limits for an encrypted folder (defaults 10 GiB and 100,000; 0 means no limit). `--max-size` also caps a file without Cryptare's format header (from v1.1.0 or earlier), which is read whole

Examples:
- cryptare decrypt ./secret.txt.enc
- cryptare decrypt ./secret.txt.enc --output ./secret.txt
- cryptare decrypt ./project-dir-backup.enc --output ./restored-project-dir
- cryptare decrypt ./secret.txt.enc --password-file ~/.config/cryptare/pw.txt

### compress

- cryptare compress [path] — compress with gzip (default) or zip
- cryptare compress [path] --output [path] — write to a custom output file or archive
- cryptare compress [path] --force — overwrite the output if it already exists
- cryptare compress [path] --format [gzip|zip] — select compression format
- cryptare compress [path] --level [1-9] — set the compression level (applies to gzip and zip; -1, the default, picks the standard level, and other values are refused)
- zip is also selected automatically when --output ends in .zip; a --format that contradicts the --output extension (gzip into `.zip`, zip into `.gz`, `.tgz` or `.tar.gz`) is refused
- compressing a single `.tar` file gives `x.tar.gz`, which `decompress` extracts as a tarball; use `decompress --raw` to get `x.tar` back

Examples:
- cryptare compress ./artifact.bin
- cryptare compress ./artifact.bin --output ./artifact.bin.gz
- cryptare compress ./build --output ./build-backup.tar.gz
- cryptare compress ./build --format zip --output ./build-backup.zip
- cryptare compress ./artifact.bin --level 9

### decompress

- cryptare decompress [archive] — decompress a gzip file or extract a tar.gz/zip archive
- cryptare decompress [archive] --output [path] — write to a custom output file or extract to a directory
- cryptare decompress [archive] --force — overwrite the output if it already exists (an existing folder is replaced, not merged into)
- cryptare decompress [archive] --max-size [size] — stop once the output passes this size (default 10 GiB; units B, KB, MB, GB, TB, KiB, MiB, GiB, TiB; 0 means no limit)
- cryptare decompress [archive] --max-entries [n] — stop if the archive has more entries than this (default 100,000; 0 means no limit)
- cryptare decompress [archive] --raw — gunzip only: write the decompressed data as one file without extracting a tar archive (`x.tar.gz` gives `x.tar`, `x.tgz` gives `x.tar`); not for zip archives. The TUI always extracts.

Examples:
- cryptare decompress ./artifact.bin.gz
- cryptare decompress ./artifact.bin.gz --output ./artifact.bin
- cryptare decompress ./build-backup.tar.gz --output ./restored-build
- cryptare decompress ./build-backup.zip --output ./restored-build
- cryptare decompress ./huge-dataset.tar.gz --max-size 50GiB
- cryptare decompress ./backup.tar.gz --raw

### keys

- cryptare keys list — list stored encryption keys
- cryptare keys path — print the key store's path (nothing is opened or created)
- cryptare keys generate — generate and store a new random encryption key
- cryptare keys export [key-id] — export a key to a file (`<key-id>-<Unix time>.ckey` in the current folder unless `--output` names one), protected by the key's own master password, which is asked for and checked
- cryptare keys import [file] — import a key from an export (only exports in the format `keys export` writes, in this or an earlier version, are accepted); the key inside must open with its master password. An export made by v1.3.1 or earlier with a separate export password needs that password too: `--export-password-file [file]`, or the prompt at a terminal (either password may go in either place)
- cryptare keys delete [key-id] — delete a stored encryption key (irreversible)
- cryptare keys delete [key-id] --yes — delete non-interactively (automation)
- cryptare keys delete [key-id] --force — delete non-interactively (automation)
- `generate`, `export` and `import` also take `--password-file [file]` (or `--password [value]`, which prints a warning); `import` also takes `--export-password-file [file]` for an older export's own password
- `export` takes `--force` to overwrite an existing output file; it never writes over the key store

Examples:
- cryptare keys list
- cryptare keys path
- cryptare keys generate --password-file ~/.config/cryptare/master.txt
- cryptare keys export 3f9a1c0e7b2d4a6f --output 3f9a1c0e7b2d4a6f.ckey
- cryptare keys import ./3f9a1c0e7b2d4a6f.ckey
- cryptare keys delete 3f9a1c0e7b2d4a6f
- cryptare keys delete 3f9a1c0e7b2d4a6f --yes

⚠️ A file encrypted with a stored key can only be decrypted with that key. Keep an export of every key you encrypt with (`keys export`) somewhere other than the key store: if the key is deleted, or the key store is lost, without an export, those files can't be recovered.

⚠️ Key deletion is permanent. Deleting a key removes it and overwrites its encrypted data in the database file, so it can't be recovered from that file. Copies made earlier (backups, synced folders, git history) are not affected, and keys deleted with v1.1.0 or earlier may still be recoverable from the file until SQLite reuses that space.

### Interactive TUI

- Running cryptare with no subcommand launches the terminal UI dashboard for interactive file and key management.
- Run `cryptare --vim` to enable vim-style TUI bindings.
- With vim bindings enabled:
  - Menus accept `j`/`k` to move, `l` to select, and `Esc`, `h`, or `b` to go back when a back action exists.
  - Forms start in insert mode so file paths and passwords can be typed normally.
  - Press `Esc` in a form to switch to normal mode, then use `j`/`k` to change fields, `h` or `Esc` to cancel, `l` or `Enter` to go to the next field (on the last field, to submit the form), and `i`/`a`/`o` to return to insert mode. `o` moves to the next field before re-entering insert mode.

## Configuration

| Setting | Default | Description |
|---|---|---|
| `CRYPTARE_DB_PATH` (environment variable) | `cryptare/cryptare.db` in your user data folder: `$XDG_DATA_HOME` (if absolute) or `~/.local/share` on Linux and other Unix systems, `~/Library/Application Support` on macOS, `%LocalAppData%` on Windows | Location of the SQLite key store, as a path or a SQLite `file:` URI. `cryptare keys path` prints the one in use. The default's `cryptare` folder is created, readable only by you, the first time the key store is needed; versions up to v1.2.0 used `cryptare.db` in the current folder instead (see the upgrade note above). A plain path can't contain `?` (SQLite would cut it there); use a `file:` URI with `%3F` instead. Only the `keys` commands (except `keys path`), the TUI, `encrypt --key` and `decrypt` of a file encrypted with a stored key open it, creating it if missing with mode 0600 (readable only by you on Linux and macOS; on Windows it gets the folder's permissions). On Linux and macOS, an existing key store, or its `-journal`, `-wal` or `-shm` file, is set to 0600 when it is opened if others can read it, and refused if another user owns it or others can write to it; run `chmod 600` on a file of your own, or point `CRYPTARE_DB_PATH` at another key store. |
| `--vim` (flag) | off | Turns on vim-style key bindings in the TUI. |

Command flags (`--output`, `--force`, `--password-file`, `--password`, `--key`, `--max-size`, `--max-entries`, `--format`, `--level`, `--raw`, `--export-password-file`, `--yes`) are described under [Core CLI capabilities](#core-cli-capabilities).

## Docker

Releases after v1.3.1 publish the image to GitHub Packages as `ghcr.io/jabbott-iii/cryptare`, tagged with the version (for example `1.4.0`), its minor line (`1.4`, which follows the newest release of that line) and `latest` (the newest release). Releases after v1.3.2 publish one image for `linux/amd64` and `linux/arm64`, so `docker pull` fetches the one for your machine, and also tag each platform's image on its own (`1.4.0-amd64`, `1.4.0-arm64`); earlier images are `linux/amd64` only. To use it without building:

```bash
docker pull ghcr.io/jabbott-iii/cryptare:latest
docker run --rm -it ghcr.io/jabbott-iii/cryptare:latest --help
gh attestation verify oci://ghcr.io/jabbott-iii/cryptare:latest --repo jabbott-iii/Cryptare   # optional: check its build provenance
```

Use the published image in the commands below in place of `cryptare:latest`, or build your own.

The image builds Cryptare with CGO enabled for the platform it is built on (`linux/amd64` or `linux/arm64`), from base images pinned by digest. It runs as an unprivileged user (UID and GID 10001), sets `CRYPTARE_DB_PATH=/app/data/cryptare.db` and declares `/app/data` as a volume owned by that user, so a named volume works as is. A bind-mounted host folder is owned by your host user instead, so run the container as that user with `--user "$(id -u):$(id -g)"`; the key store's ownership check (SEC-016) then passes too. Build with `--build-arg VERSION=<tag>` to set what `--version` reports (default `dev`).

### Build
```bash
docker build -t cryptare:latest .
```

### Run (interactive)
```bash
docker run --rm -it cryptare:latest
```

### Persist data
```bash
mkdir -p ~/.cryptare && chmod 700 ~/.cryptare
docker run --rm -it --user "$(id -u):$(id -g)" \
  -v ~/.cryptare:/app/data \
  -e CRYPTARE_DB_PATH=/app/data/cryptare.db \
  cryptare:latest
```

### CLI usage
```bash
docker run --rm -it cryptare:latest --help
docker run --rm -it --user "$(id -u):$(id -g)" -v ~/.cryptare:/app/data cryptare:latest keys list
```

## Testing and quality checks

Run these from the repository root (GNU Make and bash; `make help` lists the targets):

```bash
make build       # bin/cryptare, with CGO, stamped with `git describe`
make test        # go test ./...
make check       # a gofmt check, then CI's checks: go mod tidy, tracked key material, vet, golangci-lint, race tests, smoke test
make check-all   # plus gosec, govulncheck and the workflow linter
```

golangci-lint, gosec and govulncheck run at CI's versions, and actionlint at a pinned version, through `go run`, so there is nothing to install. See [CONTRIBUTING.md](CONTRIBUTING.md) for the full validation workflow.

## Project structure

```text
main.go                     entry point: holds the build version and calls pkg.Run
pkg/run.go                  Main (the entry point, also for programs that embed cryptare): root command, exit status, lazy key store opener
pkg/database_path.go        key store path (CRYPTARE_DB_PATH or default) and `keys path`
pkg/crypto.go               AES-256-GCM encryption, key blobs, key export/import
pkg/keys.go                 stored-key flows shared by the CLI and TUI
pkg/format_v2.go            versioned format: header, Argon2id or stored key, chunked encryption
pkg/testdata/golden/        files written by earlier versions, which the tests must keep opening
pkg/compress.go             gzip, tar.gz and zip creation and extraction
pkg/database.go             SQLite key store (GORM)
pkg/logic-cli.go            Cobra commands
pkg/ui-dashboard.go,
pkg/logic-tui.go            Bubble Tea terminal UI
CLAUDE.md, .claude/         architecture and maintenance rules (Claude Code instructions)
.github/workflows/          CI, CD, Docker and security workflows
scripts/                    third-party-licenses.sh: licence texts for release archives
```

## Contributing and license

See [CONTRIBUTING.md](CONTRIBUTING.md). Cryptare is licensed under the Apache License 2.0; see [LICENSE](LICENSE) and [NOTICE](NOTICE). Release archives also include `THIRD_PARTY_LICENSES.txt`, with the licences of the third-party code in the binaries (generated by `scripts/third-party-licenses.sh`).
