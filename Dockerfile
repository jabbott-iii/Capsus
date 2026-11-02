# Fully static Linux release binary of capsus (.claude/rules/build-and-ci.md
# P-030). cd.yml builds the "static" stage for the linux/amd64 and linux/arm64
# archives, docker.yml on every pull request, and `make docker-static` locally:
#   docker build --target static --build-arg VERSION=<version> --output type=local,dest=<dir> .
# Capsus publishes no container image (AGENTS.md D-8), so there is no runtime
# stage.
#
# The builder is pinned by digest (a multi-platform index). Its Alpine release
# is the one docker.yml and cd.yml test the binary on, which also keeps the
# build-base pin below valid. The image sets GOTOOLCHAIN=local, so its Go must
# be at least the root go.mod's go line. Change the tag, the digest and the
# package pin together.
FROM golang:1.26-alpine3.24@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS static-builder

WORKDIR /src

# Build dependencies of the CGO SQLite driver (mattn/go-sqlite3).
RUN apk add --no-cache build-base=0.5-r4

# Cache dependencies first. The root go.mod replaces each tool with its folder
# (AGENTS.md D-3), so the tools' go.mod files must be in place to load the
# build list.
COPY go.mod go.sum ./
COPY Munus/go.mod Munus/
COPY Cryptare/go.mod Cryptare/
COPY Salus/go.mod Salus/
RUN go mod download

COPY . .

# Built for the platform the image is built on: go-sqlite3 needs CGO, which the
# native toolchain doesn't cross-compile. musl instead of glibc, so the binary
# runs on any Linux distribution and no LGPL static-linking terms apply. musl
# gives new threads a 128 KiB stack unless PT_GNU_STACK asks for more; the
# tools' SQLite calls run on those threads, so the binary requests glibc's usual
# 8 MiB. go version records the toolchain in the build log.
ARG VERSION=dev
RUN go version \
    && CGO_ENABLED=1 go build \
    -tags sqlite_omit_load_extension,osusergo,netgo \
    -ldflags "-s -w -X main.version=${VERSION} -linkmode external -extldflags '-static -Wl,-z,stack-size=8388608'" \
    -o /out/capsus .

FROM scratch AS static
COPY --from=static-builder /out/capsus /capsus
