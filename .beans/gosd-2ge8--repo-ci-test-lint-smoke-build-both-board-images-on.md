---
# gosd-2ge8
title: 'Repo CI: test, lint, smoke-build both board images on every PR'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-03T17:09:32Z
parent: gosd-y0x3
---

GitHub Actions `ci.yml` on PR + main:

- [x] `go test ./...` on ubuntu-latest AND macos-latest (the pure-Go no-root promise must be enforced by CI on both)
- [x] `go vet` + golangci-lint (default linters, no bikeshedding config)
- [ ] Smoke build: `gosd build ./examples/hello` for BOTH boards using fake/cached artifacts (no hardware, no real kernel needed — use the testdata fake artifacts until the artifact pipeline lands, then switch to cached real ones), assert the .img read-back check passes (deferred: needs board-profile plumbing from gosd-3zrc; landed a cross-compile-only smoke job in the meantime)
- [ ] Cache Go modules + artifact cache between runs (Go module caching done via setup-go; artifact cache deferred alongside the image smoke build to gosd-3zrc)

## Acceptance
CI green on a trivial PR; a PR that breaks image layout fails the smoke build.

## Summary of Changes

Added `.github/workflows/ci.yml`, running on pull_request and push-to-main:
- `test`: `go test ./...` on ubuntu-latest and macos-latest (matrix)
- `vet`: `go vet ./...`
- `gofmt`: fails if `gofmt -l .` reports any files
- `lint`: golangci-lint (default linters, no config file)
- `smoke-build`: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/gosd-init ./examples/hello`, plus a host build of `./cmd/gosd`

All jobs use `actions/setup-go` with `go-version-file: go.mod` and its built-in module cache. Actions are pinned to their current major versions as of implementation (actions/checkout@v7, actions/setup-go@v6, golangci-lint-action@v9 running golangci-lint v2.12.2) rather than the older v4/v5 pins floated at planning time, since those older majors are being deprecated as GitHub Actions runners drop Node 20 support in 2026.

Fixed three pre-existing golangci-lint errcheck findings (unchecked `fmt.Fprintf`/`f.Close` errors) in `cmd/gosd-init/internal/boot/logger.go`, `examples/hello/main.go`, and `internal/build/build_test.go` so the new `lint` job starts green.

The full per-board image smoke build (`gosd build ./examples/hello` for both boards + `.img` read-back) and the artifact cache are deferred: they depend on board-profile plumbing from gosd-3zrc, which hasn't landed yet. The cross-compile smoke job proves the pure-Go, no-root, arm64 build path in the meantime.
