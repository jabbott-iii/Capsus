# Base images are pinned by digest (SEC-002); Dependabot keeps the digests
# current. Builder and runtime use the same Alpine release so the binary runs
# against the musl version it was linked with.
FROM golang:1.26-alpine3.24@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS builder

WORKDIR /src

# Install build deps for CGO sqlite3 driver
RUN apk add --no-cache build-base

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build (.dockerignore keeps local state out of the context)
COPY . .
# TARGETOS/TARGETARCH are set by BuildKit; empty values fall back to the host.
# VERSION is what salus --version reports; CD passes the release tag.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags "-X main.version=${VERSION}" -o /out/salus .

FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# OCI labels. The source label links a package in GitHub Container Registry
# to the repository. CD adds org.opencontainers.image.version and .revision.
LABEL org.opencontainers.image.title="salus" \
      org.opencontainers.image.description="Salus environment health checker: disk, memory, CPU, Docker, Kubernetes, services, time sync, certificates, and misconfigurations, reported as PASS/WARN/FAIL." \
      org.opencontainers.image.source="https://github.com/jabbott-iii/Salus" \
      org.opencontainers.image.licenses="Apache-2.0"

# No extra packages are needed at runtime: go-sqlite3 compiles SQLite into the
# binary, and Salus makes no TLS connections. Salus runs as an unprivileged
# user that owns the data directory; it must exist with that owner before the
# VOLUME instruction so new named volumes inherit it.
RUN addgroup -S -g 10001 salus \
    && adduser -S -D -H -u 10001 -G salus -h /app salus \
    && mkdir -p /app/data \
    && chown salus:salus /app/data \
    && chmod 0700 /app/data

WORKDIR /app
COPY --from=builder /out/salus /usr/local/bin/salus

USER 10001:10001

# Persist the sqlite database file (salus.db)
VOLUME ["/app/data"]

ENV SALUS_DB_PATH=/app/data/salus.db

# Salus is a CLI; arguments are passed to the salus command.
ENTRYPOINT ["salus"]
