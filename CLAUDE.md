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
go run ./cmd/api/main.go

# Build binaries
go build -o api ./cmd/api/
go build -o worker ./cmd/worker/

# Run tests
go test -v ./...

# Run a single test
go test -v -run TestName ./cmd/

# Build Docker image
docker build -t gitops-example .
```

## Architecture

Single Go binary (`cmd/api/main.go`) using the [Echo v5](https://github.com/labstack/echo) HTTP framework. The server listens on `:1323` and currently exposes one endpoint: `GET /` → `"Hello, World!"`.

The Dockerfile uses a three-stage build:
1. **build-stage** — compiles a static Linux binary (`CGO_ENABLED=0`)
2. **run-test-stage** — runs `go test` against the build stage
3. **build-release-stage** — copies only the binary into a `scratch` image, runs as UID 65532

## CI/CD

Two GitHub Actions workflows trigger automatically on push. Both push to Docker Hub under `amertz08/` and require `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` repository secrets.

**API** (`.github/workflows/docker-build-push-api.yml`) — triggers on changes to `cmd/api/main.go`:
- Pushes `amertz08/gitops-example:<sha>` and `:latest` (main) or `:<branch>-latest` (other branches)
- On main: updates the prod ArgoCD image tag
- On non-main branches: registers an ArgoCD preview app; cleans it up on branch deletion

**Worker** (`.github/workflows/docker-build-push-worker.yml`) — triggers on changes to `cmd/worker/**` or `internal/**`:
- Pushes `amertz08/gitops-example-worker:<sha>` and `:latest` (main) or `:<branch>-latest` (other branches)
- On main: updates the prod ArgoCD image tag

Both workflows use a shared reusable workflow (`.github/workflows/reusable-docker-build-push.yml`) that builds for `linux/amd64` with GitHub Actions cache.
