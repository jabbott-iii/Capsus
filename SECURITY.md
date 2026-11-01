# Security Policy

## Supported versions

Salus is maintained on a single `main` branch, with no maintenance branches.
Security fixes land on `main` and ship in the next release, so only the latest
release receives them.

| Version | Supported |
|---|---|
| Latest release on [GitHub Releases](https://github.com/jabbott-iii/Salus/releases) | :white_check_mark: |
| Earlier releases | :x: |

If you run an earlier release, upgrade to the latest one. The README has
upgrade notes for releases that change behavior. Reports against unreleased
code on `main` are also welcome; fixes land there before the next release.

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's private vulnerability
reporting:

1. On the repository's **Security & quality** tab (formerly **Security**),
   select **Report a vulnerability**, or go directly to
   <https://github.com/jabbott-iii/Salus/security/advisories/new>.
2. Fill in the form. Only you, the maintainer, and collaborators the
   maintainer adds to the report can see it.

Do not report vulnerabilities in public issues, discussions, pull requests, or
commit messages.

Include as much of the following as you can:

- The Salus version (`salus --version`), the operating system and CPU
  architecture, and how you installed Salus: a release archive, a build from
  source, the container image from `ghcr.io/jabbott-iii/salus`, or an image
  built from the `Dockerfile`.
- The command, flags, and environment variables involved (for example
  `SALUS_DB_PATH` or `SALUS_SSHD_CONFIG`).
- Steps to reproduce, or a proof of concept.
- The impact: what an attacker needs to control, and what they gain.
- A suggested fix, if you have one.

Remove secrets, credentials, host names, and other private data from any
output you attach.

## What to expect

Salus has a single maintainer, so responses are best effort. A report goes
through these steps:

1. **Acknowledgement.** The maintainer replies in the private report.
2. **Assessment.** The maintainer confirms whether it is a vulnerability,
   which versions it affects, and how severe it is, and discusses it with you
   there.
3. **Fix.** The fix is prepared privately, in the advisory's temporary private
   fork when needed, and released in a new version.
4. **Disclosure.** When the fixed release is available, the maintainer
   publishes a GitHub security advisory, credits you unless you prefer
   otherwise, and requests a CVE through GitHub when one is warranted.

If a report is declined, for example because it is out of scope, the
maintainer explains why in the report. Please keep the details private until
the advisory is published, or until you and the maintainer agree on a
disclosure date.

## Scope

In scope:

- The `salus` command and its source code in this repository, for example:
  - option or command injection into the tools Salus runs (`docker`,
    `kubectl`, `systemctl`, `timedatectl`);
  - terminal escape sequences, or forged lines or fields in JSON, Nagios,
    Prometheus, or JUnit output, caused by external tool output or other
    untrusted input;
  - disclosure of secrets that Salus reads, such as a private key in a
    `--cert` file or credentials in `DOCKER_HOST`;
  - SQL injection, or unsafe creation or permissions of the database file or
    an `--output` report file.
- The `Dockerfile` and the image built from it, including the images
  published to `ghcr.io/jabbott-iii/salus`.
- The release pipeline: the GitHub Actions workflows, release archives,
  `checksums.txt`, and build provenance attestations.
- Vulnerabilities in a dependency or in the Go toolchain that Salus code can
  reach.

Out of scope:

- Problems on the host that Salus reports, such as a finding from the
  `misconfig` check. Reporting those is Salus working as intended.
- Vulnerabilities in a dependency that Salus code cannot reach. Report those
  to the dependency's maintainers.
- Attacks that require control of the account that runs Salus, for example
  write access to the directories on its `PATH`.
- Running the container with the Docker socket mounted. The README warns
  against this because it gives the container root-equivalent control of the
  host.
- The lack of Apple or Microsoft code signing on release binaries. Verify
  downloads with `checksums.txt` and the build provenance attestation
  instead.

## Verifying releases

Every release publishes `checksums.txt`, and releases after 1.0.1 include a
signed build provenance attestation for each archive. Container images on
`ghcr.io/jabbott-iii/salus` (since 1.1.0) carry one too. See
[Release archives](README.md#release-archives) and
[Verifying build provenance](README.md#verifying-build-provenance-optional)
in the README for the commands.

## Known issues

Reported vulnerabilities are disclosed as GitHub security advisories when a
fixed release is available (see [What to expect](#what-to-expect)). Published
advisories are listed at
<https://github.com/jabbott-iii/Salus/security/advisories>.
