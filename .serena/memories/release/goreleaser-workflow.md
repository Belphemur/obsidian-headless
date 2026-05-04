# GoReleaser Release Workflow

## Overview
We use GoReleaser v2 with `dockers_v2` for multi-arch Docker manifests to build and release the obsidian-headless Go CLI.

## Binary Targets
- linux/amd64, linux/arm64
- darwin/amd64, darwin/arm64
- windows/amd64, windows/arm64

## Docker Image Targets
- linux/amd64, linux/arm64 only (Docker does not support Darwin containers)
- Base image: `alpine:latest`
- Registry: `ghcr.io/belphemur/obsidian-headless`
- Uses s6-overlay v3 for init/process supervision

## Key Files
- `.goreleaser.yml` (repo root) — GoReleaser configuration
- `.github/workflows/release.yml` — GitHub Actions release workflow
- `build/Dockerfile` — Docker image build
- `build/rootfs/` — s6-overlay service definitions
- `build/get-token.sh` — Interactive login helper

## 1Password SDK CGO Workaround
The `github.com/byteness/keyring` library imports `github.com/1password/onepassword-sdk-go`, which intentionally breaks compilation without CGO on Darwin/Linux. Since we build with `CGO_ENABLED=0` for cross-platform releases, we replace the SDK with a locally patched copy:

```go.mod
replace github.com/1password/onepassword-sdk-go => ./internal/1passwordstub
```

The patch removes an intentional compile-error symbol in `client_builder_no_cgo.go`. See `docs/onepassword-cgo-workaround.md` for full details.

## Build Info Injection
Version info is injected via ldflags into `src/internal/buildinfo`:
- `Version` — from `{{.Version}}`
- `Commit` — from `{{.Commit}}`
- `Date` — from `{{.CommitDate}}`

The CLI root command uses `cobra.Command.Version` to display this.

## Release Trigger
Pushing a git tag matching `v*` triggers the release workflow.
