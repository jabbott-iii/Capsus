<img width="1009" height="476" alt="demo" src="https://github.com/user-attachments/assets/eb426116-3d0f-4cdc-ae77-b781c7dd662b" />

# Salus

Salus is an environment health checker CLI. It verifies disk space and inodes,
memory, CPU load, Docker and Kubernetes status, Kubernetes pods, service
uptime, failed systemd units, time synchronization, certificate expiry, and
common misconfigurations, then reports a concise PASS/WARN/FAIL summary.

It is built for operators, cron jobs, CI pipelines, and monitoring systems that
need a quick, scriptable view of a host's health: results are printed as text,
JSON, Nagios plugin output, Prometheus metrics, or JUnit XML, the exit code
reflects the worst result, and each run is recorded in a local SQLite database
for later review and comparison (unless you pass `--no-save`).

## Features

- **System Resources**
  - Disk space and inode usage on one or more mount points
  - Memory and swap usage
  - CPU load: the load average relative to available CPUs (on Windows, which
    has no load average, the busy percentage of a short sample)
  - Configurable WARN and FAIL thresholds for each

- **Container Runtime**
  - Docker daemon reachability
  - Containers that fail their `HEALTHCHECK` or keep restarting

- **Orchestration**
  - Kubernetes cluster reachability, node readiness, and node Memory, Disk,
    and PID pressure via `kubectl`, for the current or a chosen kubeconfig
    context
  - Pods that are crash-looping, failed, or not Ready in a namespace

- **Services**
  - Uptime of one or more systemd services (or host uptime when no service is
    specified)
  - systemd units in the failed state
  - System clock synchronization

- **Certificates**
  - Expiry of PEM or DER certificate files, with a configurable warning window

- **Configuration**
  - Common environment misconfigurations: `HOME` not set, a database or
    kubeconfig file that other users can access, a Docker socket that every
    user can write to, `PATH` directories that every user can write to, a
    Docker TCP endpoint without TLS verification, and an SSH server that
    permits root login or password authentication

- **Job Tracking**
  - Every `check run` is recorded as a job in a local SQLite database
  - Browse past runs and their individual results, as text or JSON
  - Compare two runs, and summarize results and status changes over time
  - Delete old runs with `jobs prune`, or automatically with
    `check run --retain`

- **Scripting and monitoring**
  - Text, JSON, Nagios, Prometheus, or JUnit XML output, to standard output or
    atomically to a file
  - Exit codes that reflect the worst result, with `--fail-on` to let WARN pass

Platform support: the disk space, inode, memory, CPU load, and host uptime
checks run on Linux, macOS, and Windows:

| Platform | Disk space and inodes | Memory | CPU | Host uptime |
|---|---|---|---|---|
| Linux | `statfs` | `/proc/meminfo` (`MemAvailable`, swap) | 1-minute load average per CPU (`/proc/loadavg`) | `/proc/uptime` |
| macOS | `statfs` | VM page counts: memory other than free, speculative, file-cache, and purgeable pages, close to Activity Monitor's "Memory Used"; swap from `vm.swapusage` | 1-minute load average per CPU (`vm.loadavg`) | `kern.boottime` |
| Windows | `GetDiskFreeSpaceExW`; inodes do not apply, so `disk-inodes` reports `PASS` with a note | `GlobalMemoryStatusEx` (physical memory, and the commit charge against the commit limit) | Percentage of CPU time that was busy during a 1-second sample (`GetSystemTimes`); Windows has no load average | `GetTickCount64` |

Because `cpu-load` measures a busy percentage on Windows, its `value` (in
`--json` and `salus_check_value`) means something different there than on
Linux and macOS; keep that in mind on dashboards that mix platforms. On
Windows, a `--disk-path` that names a file is measured on its directory, as
`statfs` does on Linux and macOS.

Checking a service with `--service`, failed units, and time synchronization
require systemd, so they are Linux-only and report `WARN` elsewhere; a full
`check run` on macOS or Windows therefore exits with code `1` or higher unless
you choose checks with `--only` or pass `--fail-on fail`. Other platforms
(such as FreeBSD) report `WARN` for the resource checks too.

## Use cases

- **Scheduled host checks.** Run `salus check run --quiet` from cron, alert on
  a non-zero exit code, and review past runs with `salus jobs list` and
  `salus jobs show <id>`.
- **CI or deployment gates.** Check a build or deployment host before a
  pipeline step, for example
  `salus check run --only disk-space,memory --disk-fail 95 --json`. Treat exit
  code `2` (FAIL) as fatal and `1` (WARN) as a warning, and keep the JSON
  output with the build.
- **Service verification.** Confirm that a systemd unit is active after a
  deployment: `salus check run --only service-uptime --service nginx`.
- **Workstation checks.** Before working with containers or a cluster, confirm
  that the Docker daemon, its containers, and the cluster's nodes are healthy:
  `salus check run --only docker-status,kubernetes-status --no-save`.
- **Long-running history.** Export every past run with
  `salus jobs list --limit 0 --json`, and keep the database small under cron
  with `salus check run --retain 30d` (or `salus jobs prune --older-than 30d`).
- **Alert on change.** After each scheduled run, `salus jobs diff --exit-code`
  exits `1` only when a result changed since the previous run, and
  `salus jobs stats --since 7d` shows which checks keep changing status.
- **Prometheus.** Feed node_exporter's textfile collector from cron:
  `salus check run --format prometheus --output /var/lib/node_exporter/textfile/salus.prom --retain 30d`.
- **Nagios, Icinga, or Zabbix.** Use Salus as a plugin:
  `salus check run --format nagios --no-save`. The exit codes already follow
  the plugin convention.
- **CI reports.** Publish results as test cases with
  `salus check run --format junit --output salus-junit.xml --fail-on fail`.
- **Certificate expiry.** Warn two weeks before a certificate expires:
  `salus check run --only cert-expiry --cert /etc/ssl/certs/site.pem --cert-warn-days 14`.

## Prerequisites

To run a release binary:

- Linux, macOS, or Windows on x86-64 or ARM64 (see
  [Installation](#installation)).
- Optional tools, used by individual checks when they are on `PATH`: the
  `docker` CLI (`docker-status`), `kubectl` with a configured context
  (`kubernetes-status`, `kubernetes-pods`), `systemctl` (`service-uptime` with
  `--service`, and `systemd-failed`; Linux only), and `timedatectl`
  (`time-sync`; Linux only). A missing tool makes its check report `WARN`.
  Node readiness needs permission to list nodes; without it,
  `kubernetes-status` checks only that the cluster is reachable. An answer of
  Forbidden from the API server counts as reachable. `kubernetes-pods` needs
  permission to list pods in its namespace; without it, it reports `PASS` with
  a note.

To build from source:

- **Go 1.26.8** or newer, matching the `go` directive in `go.mod`.
- **A C toolchain** (for example `gcc` or `clang`) with `CGO_ENABLED=1`. The
  SQLite driver `github.com/mattn/go-sqlite3` uses CGO. A build with CGO
  disabled compiles but cannot open its database.
- **Git.**
- Optional: **GNU Make** for the `make` targets, **golangci-lint v2.13.2** (the
  version CI uses), and **Docker** to build the container image.

## Installation

### Release archives

Release archives and a `checksums.txt` file are published on the
[GitHub Releases](https://github.com/jabbott-iii/Salus/releases) page for each
version tag. Each archive contains a single binary with the same base name.

| Platform | Archive |
|---|---|
| Linux (x86-64) | `salus_linux_amd64.tar.gz` |
| Linux (ARM64) | `salus_linux_arm64.tar.gz` |
| macOS (Apple Silicon) | `salus_darwin_arm64.tar.gz` |
| macOS (Intel) | `salus_darwin_amd64.tar.gz` |
| Windows (x86-64) | `salus_windows_amd64.zip` |
| Windows (ARM64) | `salus_windows_arm64.zip` |

Download the archive for your platform and `checksums.txt` into the same
directory, then verify, extract, and install it.

Linux (use `linux_arm64` in place of `linux_amd64` on ARM64):
```bash
grep salus_linux_amd64.tar.gz checksums.txt | sha256sum --check
tar -xzf salus_linux_amd64.tar.gz
sudo mv salus_linux_amd64 /usr/local/bin/salus
salus --version
```

macOS (use `darwin_amd64` in place of `darwin_arm64` on Intel Macs):
```bash
grep salus_darwin_arm64.tar.gz checksums.txt | shasum -a 256 --check
tar -xzf salus_darwin_arm64.tar.gz
sudo mv salus_darwin_arm64 /usr/local/bin/salus
salus --version
```

Windows, in PowerShell (use `windows_arm64` in place of `windows_amd64` on ARM64):
```powershell
# Compare this hash with the salus_windows_amd64.zip line in checksums.txt
(Get-FileHash .\salus_windows_amd64.zip -Algorithm SHA256).Hash.ToLower()
Expand-Archive .\salus_windows_amd64.zip -DestinationPath .
Rename-Item .\salus_windows_amd64.exe salus.exe
```
Then add the directory containing `salus.exe` to your `PATH`.

### Verifying build provenance (optional)

Releases after 1.0.1 include a signed build provenance attestation for every
archive. Verify an archive with GitHub CLI 2.97 or newer (older versions have
a verification bypass, GHSA-mm27-mwq9-fr5g), replacing `vX.Y.Z` with the tag
of the release you downloaded (for example `v1.0.2`). The command needs a
GitHub login (`gh auth login`), even though the repository is public:

```bash
gh attestation verify salus_linux_amd64.tar.gz --repo jabbott-iii/Salus \
  --signer-workflow jabbott-iii/Salus/.github/workflows/cd.yml \
  --source-ref refs/tags/vX.Y.Z
```

This checks that the archive was built by this repository's release workflow
(`cd.yml`) from that tag. Verification fails for a modified archive, and for
archives from manual CD runs, which are built from a branch rather than a
release tag. The 1.0.0 and 1.0.1 archives have no build provenance
attestation.

Container images published since 1.1.0 (see [Containerization](#containerization))
carry the same kind of attestation. Verify an image by its version tag
(`X.Y.Z`, without the `v`):

```bash
gh attestation verify oci://ghcr.io/jabbott-iii/salus:X.Y.Z --repo jabbott-iii/Salus \
  --signer-workflow jabbott-iii/Salus/.github/workflows/cd.yml \
  --source-ref refs/tags/vX.Y.Z
```

### Building from source

```bash
git clone https://github.com/jabbott-iii/Salus.git
cd Salus
make build
./salus --version
```

`make build` runs `CGO_ENABLED=1 go build -o salus .`; run that command
directly if Make is not installed. A local build reports `salus version dev`.
Release builds set the version with `-ldflags "-X main.version=vX.Y.Z"`.

## Usage

### Quick start

```bash
salus check list    # list the available checks
salus check run     # run every check and record the run as a job
salus jobs list     # list recorded runs
```

Example output (values depend on the host):
```text
$ salus check run --only disk-space,memory,cpu-load,misconfig
Environment Health Check

[PASS] disk-space        /: 46.1% used (53.9% free)
[PASS] memory            memory 39.1% used, swap 8.9% used
[PASS] cpu-load          load average 1.80 across 16 CPU(s) (11% per-core)
[PASS] misconfig         no common misconfigurations detected
```

### Commands

Salus is organized into focused command groups:

- `salus check` — list and run health checks
- `salus jobs` — view past health check runs
- `salus completion` — generate a shell completion script (`bash`, `zsh`,
  `fish`, or `powershell`)
- `salus --version` — print the Salus version

#### check

- `salus check list` — list available health checks
- `salus check run` — run health checks and report the results

Examples:
```bash
salus check list
salus check run
salus check run --only disk-space,memory,cpu-load
salus check run --service nginx
salus check run --json
salus check run --fail-only
salus check run --disk-path / --disk-path /var --service nginx --service sshd
salus check run --disk-warn 70 --disk-fail 85 --timeout 10s
salus check run --only kubernetes-status,kubernetes-pods --kube-context kind-dev --kube-namespace web
salus check run --only cert-expiry --cert /etc/ssl/certs/site.pem --cert-warn-days 14
salus check run --format nagios --no-save
salus check run --format prometheus --output /var/lib/node_exporter/textfile/salus.prom
salus check run --fail-on fail --retain 30d
```

Flags for `check run`:
- `--only` — comma-separated list of checks to run (default: all; see below
  for `cert-expiry`)
- `--service` — systemd service to check (defaults to host uptime). Repeat
  the flag or separate names with commas to check several services; each gets
  its own result.
- `--disk-path` — mount path for `disk-space` and `disk-inodes` (default `/`).
  Repeat the flag to check several paths; each gets its own result. Commas are
  part of the path. On Windows, give a drive or a directory on it, such as
  `C:\` or `D:\data`; the default `/` means the root of the current drive.
- `--kube-context` — kubeconfig context for `kubernetes-status` and
  `kubernetes-pods` (defaults to kubectl's current context). A name that
  starts with `-`, contains control characters, is not valid UTF-8, or is
  longer than 253 bytes fails the check without running `kubectl`.
- `--kube-namespace` — namespace for `kubernetes-pods` (defaults to the
  context's namespace). It must be a valid namespace name: at most 63
  lowercase letters, digits, and `-`, starting and ending with a letter or
  digit. Anything else fails the check without running `kubectl`.
- `--cert` — certificate file for `cert-expiry` (PEM, possibly with a chain or
  a private key in the same file, or a single DER certificate). Repeat the
  flag for several files; each gets its own result.
- `--cert-warn-days` — days before expiry at which `cert-expiry` reports WARN
  (default `30`)
- `--disk-warn`, `--disk-fail` — disk usage percent at which `disk-space`
  reports WARN and FAIL (defaults `80` and `90`)
- `--inode-warn`, `--inode-fail` — inode usage percent at which `disk-inodes`
  reports WARN and FAIL (defaults `80` and `90`)
- `--mem-warn`, `--mem-fail` — memory usage percent at which `memory` reports
  WARN and FAIL (defaults `80` and `90`)
- `--load-warn`, `--load-fail` — 1-minute load average per CPU, in percent, at
  which `cpu-load` reports WARN and FAIL (defaults `80` and `100`; `100` means
  a load average equal to the number of CPUs). On Windows they apply to the
  busy percentage of a 1-second sample, which cannot exceed `100`: with the
  defaults, `cpu-load` warns from 80% busy and fails only when every CPU was
  busy for the whole sample.
- `--timeout` — time limit for each external command (`docker`, `kubectl`,
  `systemctl`, `timedatectl`; default `3s`). When it passes, Salus kills the
  command together with any processes it started (on Linux and macOS), and
  the result says `<command> timed out after <duration>`.
- `--check-timeout` — time limit for each check, and for each target of a
  check that runs per target (default 10 times `--timeout`, so `30s`). A check
  still running when it passes, for example `disk-space` on an unresponsive
  NFS mount, reports `FAIL` with `did not finish within <duration>`, and the
  run continues with the next check. It cannot be lower than `--timeout`.
- `--run-timeout` — time limit for the whole run (default: none). Checks it
  stops, and checks it keeps from starting, report `FAIL`.
- `--format` — report format: `text` (default), `json`, `nagios`,
  `prometheus`, or `junit` (see [Output formats](#output-formats))
- `--json` — output results as JSON (same as `--format json`; combining it
  with another `--format` is an error)
- `--output` — write the report to this file instead of standard output,
  replacing the file atomically
- `--fail-only` — only show WARN and FAIL results in text and Nagios output
- `--quiet` — suppress report output on standard output, including `--json`
  (still sets the exit code; `--output` still writes its file)
- `--fail-on` — `warn` (default) or `fail`: the lowest status that makes the
  exit code non-zero. With `fail`, a run whose worst result is WARN exits `0`.
- `--no-save` — do not persist this run to the database
- `--retain` — after saving this run and writing the report, delete saved
  runs older than this age (same format as `jobs prune --older-than`, such as
  `30d` or `12h`). The run just saved is always kept. Cannot be combined with
  `--no-save`.

Threshold values must be numbers greater than `0`, each WARN value must be
lower than its FAIL value, and disk, inode, and memory values cannot exceed
`100`. If you set a FAIL value below the default WARN value, lower the WARN
value too (for example `--disk-warn 60 --disk-fail 75`). `--timeout` takes a
duration such as `500ms`, `10s`, or `1m` and must be positive, and
`--cert-warn-days` must be a positive whole number. `--check-timeout` and
`--run-timeout` take the same durations; `0` means the default. Salus rejects
an invalid value before opening the database or running any check.

`cert-expiry` runs only when at least one `--cert` file is given: a plain
`check run` skips it, and `check run --only cert-expiry` without `--cert` is
an error (exit code `3`).

What each check reports:

| Check | PASS | WARN | FAIL |
|---|---|---|---|
| `disk-space`, `disk-inodes`, `memory`, `cpu-load` | Below the WARN threshold; for `disk-inodes`, also a filesystem that reports no inode count (such as btrfs, and every Windows volume) | At or above the WARN threshold; not Linux, macOS, or Windows; on Windows, CPU usage unknown; on Linux, memory usage unknown (`/proc/meminfo` without `MemTotal` or `MemAvailable`, or with `MemTotal` 0) | At or above the FAIL threshold; data unreadable or path missing |
| `docker-status` | Daemon reachable, no unhealthy or restarting containers | Unhealthy or restarting containers; container list unavailable; no `docker` CLI | Daemon unreachable |
| `kubernetes-status` | Cluster reachable and every node Ready without pressure, or listing nodes is forbidden | Some nodes NotReady; nodes reporting MemoryPressure, DiskPressure, or PIDPressure; readiness unknown; no `kubectl` CLI | Cluster unreachable (a Forbidden answer counts as reachable); no node Ready; invalid `--kube-context` |
| `kubernetes-pods` | Every pod Ready (completed pods are ignored), no pods, or listing pods is forbidden | Pods in CrashLoopBackOff, Failed, or not Ready; pod list unavailable; no `kubectl` CLI | Invalid `--kube-namespace` or `--kube-context` |
| `service-uptime` | Service active, or host uptime readable | Service activating or reloading; not Linux or no `systemctl`; host uptime unavailable | Service not active; invalid `--service` |
| `systemd-failed` | No failed units | Failed units (up to five are named); unit list unavailable; systemd not running; not Linux or no `systemctl` | — |
| `time-sync` | System clock synchronized | Clock not synchronized; status unknown; systemd not running; not Linux or no `timedatectl` | — |
| `cert-expiry` | Valid for at least `--cert-warn-days` more days | Expires within `--cert-warn-days` | Expired or not yet valid; file missing, unreadable, not a regular file, larger than 1 MiB, without a certificate, or with a certificate that cannot be parsed |
| `misconfig` | No problems found | One or more of the problems below | — |

Any check that does not finish within `--check-timeout`, or that
`--run-timeout` stops or keeps from starting, reports `FAIL`.

`cert-expiry` reports the certificate in the file that expires first, which in
a chain file is usually the server certificate. Messages name the certificate
and its expiry date; file contents are never printed.

The `misconfig` message lists each problem as `<rule>: <details>`, separated
by `; `. The rule identifiers are stable:

| Rule | Problem |
|---|---|
| `home-unset` | `HOME` is not set (not checked on Windows) |
| `db-permissions` | The database file is accessible by group or other users |
| `kubeconfig-permissions` | A file in `KUBECONFIG` (or `~/.kube/config`) is accessible by group or other users |
| `docker-socket-permissions` | The Docker socket (`/var/run/docker.sock`, or the `unix://` path in `DOCKER_HOST`) is writable by all users |
| `path-world-writable` | A `PATH` directory is writable by all users (an empty entry means the current directory) |
| `docker-tcp-insecure` | `DOCKER_HOST` uses `tcp://` while `DOCKER_TLS_VERIFY` is not set |
| `sshd-root-login` | The SSH server permits root login with any authentication method (`PermitRootLogin yes`) |
| `sshd-password-auth` | The SSH server accepts passwords: `PasswordAuthentication yes`, or not set at all, because OpenSSH enables it by default |

The permission rules (`db-permissions`, `kubeconfig-permissions`,
`docker-socket-permissions`, and `path-world-writable`) are not checked on
Windows, or on Windows drives that WSL mounts (drvfs, such as `/mnt/c`), whose
permission bits are not real.

The `sshd-*` rules read `/etc/ssh/sshd_config` (`%ProgramData%\ssh\sshd_config`
on Windows, or the file in `SALUS_SSHD_CONFIG`) the way sshd applies it to a
connection that matches no `Match` block: the first value of each keyword
wins, `Include` files are read in place (relative paths are taken from the
configuration file's directory), and lines after a `Match` line count only
after a `Match all` line. With `SALUS_SSHD_CONFIG` pointing outside `/etc/ssh`
(for example a host's configuration mounted into a container), absolute
`Include` paths under `/etc/ssh/` are read from that file's directory, and
any other absolute `Include` makes the rules report nothing. If
the file does not exist, or any part of the configuration cannot be read (as
for files that only root can read), these rules report nothing, so run Salus
as root for a complete check. They do not consider
`KbdInteractiveAuthentication`, which can also allow passwords through PAM.

With `--json`, `check run` prints an array with one object per result;
`duration_ns` is the check's run time in nanoseconds. Results of checks that
run once per target (`disk-space`, `disk-inodes`, `service-uptime` with
`--service`, and `cert-expiry`) carry a `target`. Results with a measured
number carry `value` and `unit` (`percent`, `seconds`, `days`, or `count`);
the other objects have neither field:
```json
[
  {
    "key": "disk-space",
    "target": "/",
    "status": "PASS",
    "message": "/: 46.1% used (53.9% free)",
    "value": 46.13,
    "unit": "percent",
    "duration_ns": 20253
  },
  {
    "key": "time-sync",
    "status": "PASS",
    "message": "system clock is synchronized",
    "duration_ns": 3150210
  }
]
```

The checks keep their positions in the array: checks added after 1.0.2 come
after `misconfig`.

The JSON shape is stable: later versions may add fields to these objects, but
do not rename or remove fields or change their types, and the output stays an
array. Ignore fields you do not know.

#### Output formats

`--format` selects the report format, and `--output <file>` writes it to a
file instead of standard output. The file is written to a temporary file in
the same directory and then renamed, so a reader never sees a partial report.
A new file is created with mode `0600`; an existing file keeps its mode and,
when the user running Salus may set it (root may), its group. To let another
user such as node_exporter read the file, create it once with the access you
need, for example `install -m 0640 -g node_exporter /dev/null salus.prom`
(or mode `0644`). The path must be a regular file in an existing directory,
not a symbolic link; Salus checks this before running any check.

- **`nagios`** — Nagios plugin output, also understood by Icinga and Zabbix.
  The first line holds the state (`OK`, `WARNING`, or `CRITICAL`, from the
  check results, so `--fail-on` applies), the result counts, and performance data
  for each result with a value, such as `'disk-space /'=46.13%`. One line per
  result follows. `|` in messages is replaced with `/`, because it separates
  performance data. If saving the run fails after the report is written, the
  report still shows the check state, but Salus exits `3`, which Nagios reads
  as UNKNOWN.

  ```text
  SALUS WARNING - 3 checks: 2 pass, 1 warn, 0 fail | 'disk-space /'=46.13% 'memory'=85.02%
  [PASS] disk-space: /: 46.1% used (53.9% free)
  [WARN] memory: memory 85.0% used, swap 0.0% used
  [PASS] time-sync: system clock is synchronized
  ```

- **`prometheus`** — the Prometheus text format, for node_exporter's textfile
  collector (which reads `*.prom` files; the temporary file name ends in
  `.tmp`). The per-check metrics have `key` and `target` labels (`target` is
  empty for checks without one):

  | Metric | Value |
  |---|---|
  | `salus_check_status` | `0` PASS, `1` WARN, `2` FAIL (not affected by `--fail-on`) |
  | `salus_check_value` | The measured value; the `unit` label names its unit |
  | `salus_check_duration_seconds` | How long the check took |
  | `salus_last_run_timestamp_seconds` | When the run finished (Unix time), to alert on stale results |

  Messages are left out, because their text changes between runs.

- **`junit`** — JUnit XML with one test case per result, named after the check
  and target. FAIL results are failures. WARN results are failures with the
  default `--fail-on warn`, and passing test cases with `--fail-on fail`, so
  the report agrees with the exit code.

#### jobs

- `salus jobs list` — list recent health check runs
- `salus jobs show [job-id]` — show details for a specific run
- `salus jobs prune --older-than <age>` — delete runs that started longer ago
  than `<age>`, with their results
- `salus jobs diff [from-id] [to-id]` — show which results changed between two
  runs. With no ids, it compares the two most recent runs; with one id, that
  run and the most recent run.
- `salus jobs stats` — count PASS, WARN, and FAIL results and status changes
  per check over recent runs

Examples:
```bash
salus jobs list
salus jobs list --limit 50 --json
salus jobs show 7
salus jobs show 7 --json
salus jobs prune --older-than 30d --dry-run
salus jobs prune --older-than 12h
salus jobs diff
salus jobs diff 7 --exit-code
salus jobs stats --since 7d --flap-threshold 5
```

Flags:
- `jobs list --limit` — maximum number of jobs to list (default `20`; `0`
  lists all)
- `jobs list --json`, `jobs show --json` — output as JSON
- `jobs prune --older-than` — required age: a whole number of days such as
  `30d`, or a duration such as `12h` or `90m`
- `jobs prune --dry-run` — report how many runs would be deleted, without
  deleting them
- `jobs diff --exit-code` — exit with `1` when any result changed, and `0`
  when none did (like `git diff --exit-code`)
- `jobs diff --json`, `jobs stats --json` — output as JSON
- `jobs stats --since` — summarize runs that started within this age (default
  `7d`; same format as `--older-than`)
- `jobs stats --flap-threshold` — status changes at which a check is flagged
  as flapping (default `3`)

`jobs list --json` prints an array of jobs with `id`, `status`, `started_at`,
`finished_at` (`null` if the run never finished), and `summary`.
`jobs show --json` prints one job with the same fields plus `results`, whose
objects have the `check run --json` shape (`duration_ns` comes from a stored
value with millisecond precision). Pruning frees space inside the database
file for new runs; the file itself does not shrink.

`jobs diff` matches results by check and target. It lists each result whose
status changed (marked as the old and new status, for example
`[PASS -> WARN]`), results only in the newer run (`added`), and results only
in the older run (`removed`), each followed by its message (from the newer
run, or from the older run for `removed` results).
`jobs diff --json` prints `from` and `to` (job objects as above), `changes`
(objects with `key`, `target` when there is one, `change` — `worse`,
`better`, `added`, or `removed` — `from` unless the result was added, `to`
unless it was removed, and `message`), and the `unchanged` count. With one
job id, that id must not be the most recent run.

`jobs stats` counts, for each check and target, the runs that included it,
its PASS, WARN, and FAIL results, its status changes between consecutive runs
that included it, and its latest status. A check is flagged as flapping when
its status changed at least `--flap-threshold` times. `jobs stats --json`
prints `since`, `runs`, and `checks` (objects with `key`, `target` when
there is one, `runs`, `pass`, `warn`, `fail`, `changes`, `last_status`, and
`flapping`).

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Every check passed, or the command succeeded |
| `1` | At least one check reported WARN, and none reported FAIL |
| `2` | At least one check reported FAIL |
| `3` | Salus could not complete the command |

`check run` uses codes `0` to `2`, which makes it suitable for scripts and CI
pipelines. With `--fail-on fail`, a run whose worst result is WARN exits `0`.
These codes match the Nagios plugin convention (`3` is UNKNOWN there). Any
command exits with code `3` for invalid flags, arguments, or threshold values
(including an unknown command or subcommand), an unknown check name in
`--only`, a job that does not exist, a database error, or an `--output` file
that cannot be written. The exceptions are `salus help <topic>` and
`salus completion <shell>`: an unknown topic or shell prints help on stdout
and exits `0`. `jobs diff --exit-code` exits `1` when results changed. Errors
are written to stderr, so stdout carries only command output (for example,
clean JSON with `--json`).

`check run` writes its report before it saves the run, so if saving fails
(for example when another process holds the database lock for more than five
seconds, or the disk is full), the report is still written, and Salus exits
`3`. If `check run` receives `SIGINT` (Ctrl-C) or `SIGTERM` while checks are
running, it stops them and the commands they started, writes no report,
saves nothing, and exits `3`. A second signal ends it at once. A signal that
the process inherited as ignored, such as `SIGINT` for a background job of a
non-interactive shell, stays ignored. An internal error (a bug in Salus) is
reported on stderr, also with exit code `3`.

## Configuration

Salus stores job history in a SQLite database. The database is created on the
first command that needs it (`check run` without `--no-save`, `check list`,
`jobs list`, `jobs show`, `jobs prune`, `jobs diff`, `jobs stats`); `--help`,
`--version`, `completion`, and `check run --no-save` never create it.

| Setting | Default | Purpose |
|---|---|---|
| `SALUS_DB_PATH` (optional) | Per-user location below | Path of the database file. Overrides the default. |
| `SALUS_SSHD_CONFIG` (optional) | `/etc/ssh/sshd_config` (`%ProgramData%\ssh\sshd_config` on Windows) | The sshd configuration that the `sshd-*` misconfig rules read, for example a host's configuration mounted into a container. |

Default database location when `SALUS_DB_PATH` is not set:

| Platform | Path |
|---|---|
| Linux and other Unix | `$XDG_DATA_HOME/salus/salus.db`, or `~/.local/share/salus/salus.db` when `XDG_DATA_HOME` is unset or not an absolute path |
| macOS | `~/Library/Application Support/salus/salus.db` |
| Windows | `%LOCALAPPDATA%\salus\salus.db` |

Salus creates a new database file with mode `0600` and any missing parent
directories with mode `0700`. The `misconfig` check warns if the database file
is accessible by group or other users (on Linux and macOS).

Several Salus processes can use the same database at once, for example two
cron entries that start in the same minute. A process waits up to five
seconds for another one's write to finish. `SALUS_DB_PATH` may end with
go-sqlite3 connection parameters, such as `?_busy_timeout=10000` for a longer
wait; Salus adds `_busy_timeout=5000` and `_txlock=immediate` unless the path
sets them.

Check thresholds, targets, and the command timeout are set for each run with
the `check run` flags above. There is no configuration file.

### Upgrading from 1.0.2

These changes can raise the exit code on hosts that passed before:

- `docker-status` reports `WARN` when containers are unhealthy or restarting.
- `kubernetes-status` checks node readiness: `WARN` when some nodes are
  NotReady, and `FAIL` when no node is Ready. It also reports `WARN` for nodes
  under Memory, Disk, or PID pressure.
- `misconfig` also checks kubeconfig files, the Docker socket, `PATH`
  directories, a Docker TCP endpoint without TLS verification, and the SSH
  server configuration. Every problem now starts with its rule identifier (for
  example `db-permissions: ...`), so scripts that match `misconfig` messages
  must allow for the prefix. Hosts with an OpenSSH server configuration
  (`/etc/ssh/sshd_config`) in its stock form usually warn about
  `sshd-password-auth`, because OpenSSH accepts passwords unless
  `PasswordAuthentication no` is set. The rule reads the configuration file,
  so it can also warn where the server is installed but not running (for
  example macOS with Remote Login turned off).
- A plain `check run` also runs `disk-inodes`, `kubernetes-pods`,
  `systemd-failed`, and `time-sync`. Without systemd (containers, macOS,
  Windows) or `kubectl`, they report `WARN`; use `--only` to choose checks, or
  `--fail-on fail` to let WARN pass.
- On macOS and Windows, the disk space, inode, memory, CPU, and host uptime
  checks now measure the host instead of always reporting `WARN`, so they can
  pass, or report `WARN` or `FAIL` from the thresholds. A plain `check run`
  on Windows takes about a second longer, for the CPU sample.

Other changes:

- Repeating `--disk-path` or `--service` now checks every value; 1.0.2 used
  only the last one. Scripts that append a user override to a default (for
  example `salus check run --disk-path / "$@"`) now check both paths.
- JSON results can carry `target`, `value`, and `unit` fields, and a plain
  `check run` returns more results; existing checks keep their positions.
- Results recorded by 1.0.2 have no target. `jobs diff` and `jobs stats` match
  them to the new results when the old message names the target and the check
  has a single target (one `--disk-path` or `--service` value), so the first
  comparison after the upgrade does not report every disk or service result
  as removed and added.
- The first command after the upgrade that opens the database adds three
  columns to its results table and the new checks to its catalog, and records
  the schema version in the database, so it needs write access to the
  database file once. A database that is already current still opens
  read-only.
- `check run` writes the report before saving the run. When saving fails,
  the report (and an `--output` file) is still written; the exit code is `3`,
  as before.
- A check that hangs is stopped after `--check-timeout` (default `30s`) and
  reports `FAIL`. External commands now stop at `--timeout` even when they
  started other processes that keep running, such as kubectl credential
  plugins; on Linux and macOS those processes are killed with the command.
  Commands run in their own process group there, so a command that tries to
  prompt on the terminal (for example ssh asking for a key passphrase for an
  `ssh://` `DOCKER_HOST`) is stopped and times out. A command that prints more
  than 8 MiB of output counts as failed: its check reports `WARN` or `FAIL`, as
  for any other error of that command.
- `SIGINT` and `SIGTERM` during `check run` end it with exit code `3`,
  without a report or a saved run (1.0.2 was killed by the signal).
- On Linux, `memory` reports `WARN` ("memory usage unknown") when
  `/proc/meminfo` has no `MemTotal` or `MemAvailable` (kernels before 3.14),
  instead of `FAIL` or `PASS` from a made-up value.

### Upgrading from 1.0.0

- **Database location:** 1.0.0 created `salus.db` in the current working
  directory. Since 1.0.1, Salus no longer reads that file by default. To keep
  your job history, move it to the location above, or set
  `SALUS_DB_PATH=/path/to/salus.db`.
- **Database permissions:** databases created by 1.0.0 are usually readable by
  other users (mode `0644`), which now makes `misconfig` report `WARN` (exit
  code `1`). Restrict the file with `chmod 600 /path/to/salus.db`.
- **`--service` values** must be systemd unit names (letters, digits, and
  `:-_.\@`, not starting with `-`). Other values fail the `service-uptime`
  check without running `systemctl`.
- **Docker volumes:** the 1.0.0 image ran as root, so a database it created in
  the `/app/data` volume is owned by root. Since 1.0.1 the image runs as UID
  10001, so hand the existing data over once:

  ```bash
  docker run --rm --user 0 --entrypoint sh -v salus-data:/app/data salus \
    -c 'chown -R 10001:10001 /app/data && chmod 700 /app/data && find /app/data -type f -exec chmod 600 {} +'
  ```

## Containerization

Each release since 1.1.0 publishes a container image for `linux/amd64` and
`linux/arm64` to the GitHub Container Registry. Pull it by version (without
the `v` of the release tag):
```bash
docker pull ghcr.io/jabbott-iii/salus:X.Y.Z
```

| Tag | Points to |
|---|---|
| `X.Y.Z` | That release |
| `X.Y` | The most recently published stable release of that minor version |
| `latest` | The most recently published stable release |
| `X.Y.Z-amd64`, `X.Y.Z-arm64` | One architecture of that release |

Pre-releases (tags such as `v1.2.0-rc.1`) get only their own version tags
(`1.2.0-rc.1`, `1.2.0-rc.1-amd64`, and `1.2.0-rc.1-arm64`) and never move
`X.Y` or `latest`. In the image, `salus --version` reports the release tag,
for example `salus version v1.1.0`. To check that an image was built by this
repository's release workflow, see
[Verifying build provenance](#verifying-build-provenance-optional).

To build the image yourself from the multi-stage `Dockerfile` instead:
```bash
docker build -t salus .
```
A local build reports `salus version dev` unless you pass
`--build-arg VERSION=<version>`.

Run a health check (with a local build, use `salus` as the image name):
```bash
docker run -it --rm \
  -v salus-data:/app/data \
  -e SALUS_DB_PATH=/app/data/salus.db \
  ghcr.io/jabbott-iii/salus:X.Y.Z check run
```

Note:
 - The container runs as the unprivileged user `salus` (UID and GID `10001`)
   and uses `SALUS_DB_PATH=/app/data/salus.db` by default.
 - Database state is persisted in `/app/data`. A new named volume, as in the
   example above, is writable by the container automatically. For a bind
   mount, make the host directory writable by UID `10001` first (for example
   `sudo chown 10001:10001 /path/to/data`). A volume created by Salus 1.0.0
   needs a one-time ownership fix (see
   [Upgrading from 1.0.0](#upgrading-from-100)).
 - The `docker-status`, `kubernetes-status`, `kubernetes-pods`,
   `systemd-failed`, and `time-sync` checks, and `service-uptime` with
   `--service`, are not supported inside the container. The image does not include the `docker`, `kubectl`,
   `systemctl`, or `timedatectl` tools, so those checks report `WARN` (for
   example `... CLI not found in PATH`). Run the `salus` binary on the host
   for them. Do not mount the Docker socket into the container: it gives the
   container root-equivalent control of the host.
 - Resource checks inside a container see the container's view: `/proc`
   memory and load figures are host-wide, and `disk-space` measures the
   container filesystem unless you mount a host path and pass `--disk-path`.

## Testing and quality checks

Run these from the repository root. Each `make` target wraps the command shown.

| Command | Runs |
|---|---|
| `make test` | `go test ./...` |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run ./...` three times, with `GOOS=linux`, `GOOS=darwin`, and `GOOS=windows` |
| `make fmt` | `gofmt -s -w .` |
| `make cover` | `go test -coverprofile=coverage.out ./...`, then `go tool cover -func=coverage.out` |
| `make build` | `CGO_ENABLED=1 go build -o salus .` |

`make lint` expects golangci-lint on `PATH`; point it at another binary with
`make lint GOLANGCI_LINT=/path/to/golangci-lint`. Also run
`go test -race ./...` for concurrency-related changes, and
`go mod tidy && git diff --exit-code`, because CI fails when `go.mod` or
`go.sum` drift.

CI runs `go vet`, golangci-lint (with the linters pinned in `.golangci.yml`),
and the tests on Linux, macOS, and Windows, and smoke-tests the built binary.
The Security workflow runs CodeQL, gosec, and govulncheck. See
[CONTRIBUTING.md](CONTRIBUTING.md) for the full checklist before opening a
pull request.

## Project structure

```text
.
├── main.go               Entry point: holds the build version, exits with pkg.Main's code
├── pkg/                  All application code (one Go package, package pkg)
│   ├── main.go           Main: runs the CLI, maps results to exit codes, --version
│   ├── version.go        Root command with the build version attached
│   ├── logic-cli.go      Cobra commands and flags
│   ├── health*.go        Check registry, thresholds, and the individual checks
│   ├── report.go         Text and JSON output, exit codes
│   ├── report-formats.go Nagios, Prometheus, and JUnit output; atomic --output files
│   ├── report-files_*.go Keeps an existing --output file's group (per OS)
│   ├── database*.go      Database location, file permissions, GORM models
│   ├── scan-store.go     Job and result storage
│   ├── scan-history.go   jobs diff and jobs stats
│   └── seed.go           Built-in check catalog
├── .github/              CI, security scanning, Docker smoke tests, releases, Dependabot,
│                         issue and pull request templates
├── Dockerfile            Multi-stage container image (runs as UID 10001)
├── Makefile              Development targets and release tagging
├── .golangci.yml         Pinned golangci-lint linter set
├── CLAUDE.md, .claude/   Maintainer guidance for Claude Code: rules, skills, permissions
├── CONTRIBUTING.md       Contribution workflow and validation
├── SECURITY.md           Supported versions and private vulnerability reporting
└── LICENSE, NOTICE       Apache-2.0 license and third-party notices
```

## Contributing and license

Contributions start with an issue; see [CONTRIBUTING.md](CONTRIBUTING.md).
Report security vulnerabilities privately, as described in
[SECURITY.md](SECURITY.md), not in a public issue.
Salus is licensed under the Apache License 2.0 (see [LICENSE](LICENSE)), and
third-party notices are in [NOTICE](NOTICE).
