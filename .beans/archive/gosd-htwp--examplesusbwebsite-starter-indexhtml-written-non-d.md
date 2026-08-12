---
# gosd-htwp
title: 'examples/usbwebsite: starter index.html written non-durably and never repaired if truncated'
status: completed
type: bug
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-08-07T16:18:17Z
---

Found by review sweep `gosd-fuxs` (cross-cutting area), verified.

ensureStarterPage (examples/usbwebsite/main.go:364-381) does a bare
os.WriteFile to the FAT volume, guarded by an existence check — so a
power cut mid-write leaves a truncated/empty index.html that the
existence check then treats as user content and never rewrites. Every
other write in the examples follows docs/runtime.md's "Making a write
durable" sequence (hello and emmcstorage both carry writeFileDurably).

**Fix:** reuse the durable-write sequence for the starter page (worth
factoring the helper into a small shared spot for examples, or accept the
duplication as the examples' stdlib-only style does elsewhere).



## Summary of Changes

- `ensureStarterPage` (examples/usbwebsite/main.go) now writes the starter
  index.html with the write→fsync→rename→fsync-dir sequence from
  docs/runtime.md's "Making a write durable" (`writeFileDurably`/`syncDir`,
  following the same duplicated-per-example pattern as `examples/hello`,
  `examples/emmcstorage`, and `examples/diskstorage`).
- On startup it now detects a truncated or empty index.html — the exact
  debris a power cut mid-write of the old bare `os.WriteFile` could leave —
  and repairs it by re-writing the embedded starter durably. Detection
  (`isTruncatedStarter`) treats content as corrupt only when it is a strict
  prefix of the starter page (empty included); anything else, including the
  complete starter or genuine unrelated content, is left untouched.
- Added behavioral tests in `main_test.go`: `TestIsTruncatedStarter` for the
  prefix-detection logic, and `TestEnsureStarterPage` covering no file,
  empty file, truncated file, and real user content, asserting on the
  resulting index.html contents (and that no stray `.tmp` file survives a
  successful write).
- Verified: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint
  run ./...` (host and `GOOS=linux`) all clean; cross-compiled
  `examples/usbwebsite` for `GOARCH=arm64` and `GOARCH=arm GOARM=6`.
