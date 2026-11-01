# Builder: the same Go release as the toolchain line in go.mod (SEC-018), pinned by
# digest (SEC-013). Change the tag and digest together with go.mod.
FROM golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS builder

WORKDIR /src

# Install build deps for CGO sqlite3 driver
RUN apk add --no-cache build-base

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build. VERSION is what --version reports (default "dev"). The
# binary is built for the platform the image is built on (go-sqlite3 needs CGO, which
# doesn't cross-compile with the native toolchain): CD builds linux/amd64 and
# linux/arm64 on runners of each.
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags "-X main.version=${VERSION}" -o /out/cryptare .

# Runtime, pinned by digest (SEC-013); both base images' digests are multi-platform. No packages are added: go-sqlite3 compiles
# SQLite into the binary, and Cryptare makes no network connections.
FROM alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8

# An unprivileged user that owns the data folder (SEC-013). A new named volume
# takes this ownership; for a bind mount, run with --user "$(id -u):$(id -g)".
RUN addgroup -S -g 10001 cryptare \
 && adduser -S -D -H -u 10001 -G cryptare cryptare \
 && mkdir -p /app/data \
 && chown cryptare:cryptare /app/data \
 && chmod 0700 /app/data

WORKDIR /app
COPY --from=builder /out/cryptare /usr/local/bin/cryptare

# Persist sqlite database file (cryptare.db)
VOLUME ["/app/data"]

ENV CRYPTARE_DB_PATH=/app/data/cryptare.db

USER 10001:10001

# This app is an interactive TUI/CLI
ENTRYPOINT ["cryptare"]
