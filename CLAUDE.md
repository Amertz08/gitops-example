# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Setup

After cloning, activate the git hooks:

```bash
git config core.hooksPath .githooks
```

Requires `golines` (`go install github.com/segmentio/golines@latest`). The pre-commit hook runs golines at max line length 100 on staged Go files.

## Commands

```bash
# Run locally
go run ./cmd/main.go

# Build binary
go build -o api ./cmd/

# Run tests
go test -v ./...

# Run a single test
go test -v -run TestName ./cmd/

# Build Docker image
docker build -t gitops-example .
```

## Architecture

Single Go binary (`cmd/main.go`) using the [Echo v5](https://github.com/labstack/echo) HTTP framework. The server listens on `:1323` and currently exposes one endpoint: `GET /` → `"Hello, World!"`.

The Dockerfile uses a three-stage build:
1. **build-stage** — compiles a static Linux binary (`CGO_ENABLED=0`)
2. **run-test-stage** — runs `go test` against the build stage
3. **build-release-stage** — copies only the binary into a `scratch` image, runs as UID 65532

## CI/CD

The GitHub Actions workflow (`.github/workflows/docker-build-push.yml`) is **manual-trigger only** (`workflow_dispatch`). It builds and pushes the image to Docker Hub under `amertz08/gitops-example` tagged with both the commit SHA and `latest`. Requires `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` repository secrets.
