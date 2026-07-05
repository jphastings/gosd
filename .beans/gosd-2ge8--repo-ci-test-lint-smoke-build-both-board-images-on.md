---
# gosd-2ge8
title: 'Repo CI: test, lint, smoke-build both board images on every PR'
status: completed
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-05T05:34:01Z
parent: gosd-y0x3
---

GitHub Actions `ci.yml` on PR + main:

- [x] `go test ./...` on ubuntu-latest AND macos-latest (the pure-Go no-root promise must be enforced by CI on both)
- [x] `go vet` + golangci-lint (default linters, no bikeshedding config)
- [x] Smoke build: `gosd build ./examples/hello` for BOTH boards using fake/cached artifacts (landed as the `image-smoke-build` job, added alongside gosd-wtpa: `gosd build ./examples/hello` with no `--board` and `--artifacts-dir cmd/gosd/testdata/fake-artifacts` builds both boards in one invocation; the .img read-back/content assertions already live in cmd/gosd's `go test` integration tests, so this job asserts the CLI exits 0 and both images are non-empty)
- [x] Cache Go modules + artifact cache between runs (Go module caching done via setup-go's built-in cache on every job; the internal/artifacts download cache intentionally isn't exercised in CI — the image-smoke-build job builds via --artifacts-dir against committed fake artifacts precisely so CI needs no network access or a published artifact release, so there's nothing for that cache to hold here, see the job's comment in ci.yml)

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

## Summary of Changes (2026-07-05, gosd-wtpa follow-up)

Landed the two todos deferred pending gosd-3zrc/gosd-wtpa:

- Added the `image-smoke-build` job to `.github/workflows/ci.yml`: runs
  `go run ./cmd/gosd build ./examples/hello --artifacts-dir
  cmd/gosd/testdata/fake-artifacts -o dist` (no `--board`, so both
  registered boards build in one invocation per the locked "no --board
  builds all boards" decision) and asserts both `.img` files exist and are
  non-empty. The .img read-back/content assertions themselves already live
  in `cmd/gosd`'s `go test` integration tests
  (`TestBuildProducesABootableImageFromFakeArtifacts` and the radxa-zero-3e
  counterpart), so this job only needs to prove the CLI command exits 0 in
  CI — verified by running the exact command locally before landing it.
  The original `smoke-build` job (cross-compile only) stays as a narrower,
  faster check.
- Go module caching was already handled by `actions/setup-go`'s built-in
  cache on every job. The `internal/artifacts` download cache doesn't apply
  to `image-smoke-build`: it deliberately uses `--artifacts-dir` with
  committed fake artifacts so this job needs no network access or a
  published `artifacts/vX.Y.Z` release — there's nothing for that cache to
  hold here. A cache keyed on `internal/artifacts.Version` would only be
  worth adding to a future job that builds from a real release.

All todos are now checked; marking this bean completed.
