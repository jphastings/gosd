---
# gosd-wo0l
title: 'examples/hello: never logs which port it bound — silent :80→:8080 fallback confuses serial-console debugging'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-08-07T16:20:25Z
---

Found by review sweep `gosd-fuxs` (cross-cutting area), verified.

examples/hello/main.go:44-48 falls back from :80 to :8080 with no log
line; the startup banner prints before Listen. Someone watching the
serial console has no way to know the app moved ports (docs and examples
assume :80).

**Fix:** log listener.Addr() after Listen succeeds.

## Todos

- [x] Log the fallback event itself, with why (":80 unavailable (%v); falling back to :8080")
- [x] Log the bound port via listener.Addr() after Listen succeeds
- [x] Keep stdlib-only; verify cross-compile for arm64 and GOARCH=arm GOARM=6
- [x] Check README/docs for fallback wording that needs to stay consistent
- [x] Quality gates: go test ./..., go vet ./..., gofmt -l ., golangci-lint (darwin + GOOS=linux)
- [x] Open PR

## Summary of Changes

`examples/hello/main.go`'s `main()` now logs both the fallback event and the
port actually bound. When `:80` fails, it prints
`gosd hello: :80 unavailable (%v); falling back to :8080` (actionable,
matches the `X failed because Y` convention) before retrying; once `Listen`
succeeds (on either port) it prints `gosd hello: listening on <addr>` using
`listener.Addr()`, so a serial console shows the real bound port instead of
staying silent. Both lines go to stdout via `fmt.Printf`, matching the
existing startup banner's stream — only the pre-existing "both ports
failed"/"server stopped" errors stay on stderr.

Checked `README.md` and `docs/runtime.md`/`docs/*.md` for fallback wording:
README.md's "See `examples/hello`... falls back to `:8080` if `:80` is
unavailable" already describes the behavior generically (not a literal log
line) and needed no change. `docs/runtime.md`'s `:8080` mentions are about
`gosd run`'s host-port forwarding for qemu-virt, an unrelated mechanism.

Verified stdlib-only and cross-compiling for both board arches:
`GOOS=linux GOARCH=arm64 go build ./examples/hello` and
`GOOS=linux GOARCH=arm GOARM=6 go build ./examples/hello`.

All quality gates pass: `go test ./...`, `go vet ./...`, `gofmt -l .`
(clean), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`
(0 issues, both). Note: this bench machine was running many concurrent
sibling agents during verification, causing transient shared-disk ENOSPC
and go-build-cache contention (per CLAUDE.md's documented "bizarre build
failures" note) — every affected package was re-run in isolation once
contention cleared and passed cleanly; none of the transient failures were
related to this change.
