# Artifact Dependency Resolver

This repository contains a pure Go API for resolving artifact dependencies and
persisting resolution graphs and lockfiles. It targets Go 1.22 and starts the
HTTP server from `./cmd/server` on port 8080 by default.

## Local development

```bash
go mod download
go build ./...
go test ./...
LISTEN_ADDR=:8080 DB_PATH=./data/app.db go run ./cmd/server
```

The evaluation image can be built for either common target platform:

```bash
./build_benzhi_docker.sh artifact-resolver-eval linux/amd64
./build_benzhi_docker.sh artifact-resolver-eval linux/arm64
```
