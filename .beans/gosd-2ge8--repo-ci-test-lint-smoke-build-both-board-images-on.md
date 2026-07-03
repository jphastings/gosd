---
# gosd-2ge8
title: 'Repo CI: test, lint, smoke-build both board images on every PR'
status: todo
type: task
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-02T21:10:00Z
parent: gosd-y0x3
---

GitHub Actions `ci.yml` on PR + main:

- [ ] `go test ./...` on ubuntu-latest AND macos-latest (the pure-Go no-root promise must be enforced by CI on both)
- [ ] `go vet` + golangci-lint (default linters, no bikeshedding config)
- [ ] Smoke build: `gosd build ./examples/hello` for BOTH boards using fake/cached artifacts (no hardware, no real kernel needed — use the testdata fake artifacts until the artifact pipeline lands, then switch to cached real ones), assert the .img read-back check passes
- [ ] Cache Go modules + artifact cache between runs

## Acceptance
CI green on a trivial PR; a PR that breaks image layout fails the smoke build.
