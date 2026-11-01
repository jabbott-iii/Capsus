<!--
Pull requests need a confirmed issue (CONTRIBUTING.md, "Contribution workflow").
Keep the pull request focused on that issue.
-->

## Linked issue

Closes #

## What changed and why

## Validation

<!-- Tick what you ran, and list results or anything you could not run. -->

- [ ] `gofmt -s -w .` (`make fmt`) leaves no changes
- [ ] `go vet ./...`
- [ ] `go test ./...`, and `go test -race ./...` for concurrency-related changes
- [ ] `make lint` (golangci-lint for `GOOS=linux`, `darwin`, and `windows`)
- [ ] `go mod tidy && git diff --exit-code`

## Checklist

- [ ] Tests cover the change.
- [ ] Documentation is updated where the change affects it (see CONTRIBUTING.md, "Documentation").
- [ ] Public contracts (see CLAUDE.md, "Public contracts") are unchanged, or the change was approved on the issue.
- [ ] No secrets, credentials, tokens, local databases (`*.db`), or `.env` files are included.

## Security notes

<!-- Call out security-sensitive changes for review, or write "None".
Do not describe an undisclosed vulnerability here; report it privately as
described in SECURITY.md. -->
