# Security Policy

Cryptare encrypts people's files, so security reports are welcome and taken seriously.

## Supported versions

Only the latest minor release line gets security fixes, shipped as its next patch
release. When a new minor version is released, it takes the place of the one below.

| Version | Supported |
| ------- | --------- |
| 1.4.x   | Yes       |
| < 1.4   | No        |

v1.0.0 doesn't run at all (see the README); upgrade from any unsupported version to the
latest release.

## Reporting a vulnerability

Please don't report a vulnerability in a public issue, pull request or discussion.

Report it privately through GitHub's private vulnerability reporting: on the
repository's **Security** tab, choose **Report a vulnerability**
(<https://github.com/jabbott-iii/Cryptare/security/advisories/new>). Only the maintainer
sees the report.

A useful report includes:

- the Cryptare version (`cryptare --version`) and your operating system;
- what is affected: the encryption format, password or key handling, the key store,
  archive extraction, the release binaries or workflows, or something else;
- steps or a proof of concept that reproduce the issue, and what an attacker gains.

Don't include real passwords, keys or private files; a made-up example that shows the
problem is enough.

## What happens next

- You get an acknowledgement within 7 days.
- The report is assessed, and if it is confirmed, a fix and its timeline are agreed with
  you for that report.
- The fix ships in a patch release of the supported line, with a GitHub security
  advisory describing the issue. You are credited in it unless you prefer not to be.
- Please keep the details private until the fix is released.
