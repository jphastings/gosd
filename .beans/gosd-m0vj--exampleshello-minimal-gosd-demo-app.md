---
# gosd-m0vj
title: 'examples/hello: minimal GoSD demo app'
status: completed
type: task
priority: normal
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-03T07:34:02Z
parent: gosd-vi0n
---

A single-file main package used by every test and hardware validation.

Behavior: print "gosd hello, host=<hostname> board=<GOSD_BOARD env>" to stdout at startup, then serve HTTP on :80 responding with hostname, uptime, and the request remote address. Only stdlib. No flags. Must run fine as a normal process on the dev machine too (`go run ./examples/hello` serves on :80 or falls back to :8080 if :80 is denied).

- [x] Write it
- [x] go vet clean, gofmt clean

## Acceptance
`go run ./examples/hello` responds on localhost; binary cross-compiles with CGO_ENABLED=0 GOOS=linux GOARCH=arm64.

## Summary of Changes

Replaced the scaffold placeholder with the real examples/hello app: stdlib-only, no flags. On startup it prints `gosd hello, host=<hostname> board=<GOSD_BOARD env>` to stdout, then listens on :80 (falling back to :8080 if :80 can't be bound) and serves a single handler reporting hostname, uptime, and the request's remote address.

Verified with `go run ./examples/hello` + curl (both bare and with GOSD_BOARD set), and cross-compiled with CGO_ENABLED=0 GOOS=linux GOARCH=arm64 to confirm it still builds for the target board.
