---
# gosd-w83z
title: 'qemurun: -drive file= value doesn''t escape commas per QEMU option syntax'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-08-07T16:04:10Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

qemurun.go:195 builds `if=none,file=<path>,format=raw,id=hd0` by
concatenation; QEMU requires literal commas in values to be doubled.
gosd-generated paths never contain commas, but `gosd qemuboot <img>` takes
a user path — a comma-bearing path misparses (error, or wrong file
attached).

**Fix:** double literal commas in imgPath before building the -drive
value.



## Summary of Changes

- Added `escapeOptionValue` (`internal/qemurun/qemurun.go`), which doubles
  literal commas per QEMU's own option-value escaping rule, and applied it to
  the `imgPath` interpolated into the `-drive if=none,file=...` value built
  by `Args`. `imgPath` is the one qemurun-built value that can carry
  attacker/user-chosen content (via `gosd qemuboot <img>`); a literal comma
  in it previously misparsed the `-drive` option (extra bogus key=value pair,
  or attaching the wrong file).
- Audited every other interpolated value in `Args`: `-kernel`/`-initrd` take
  a plain filename (not QEMU's comma-delimited option syntax), so workDir
  needs no escaping regardless of its origin; `-netdev`'s hostfwd port and
  `-m`'s memory size are formatted from `int`s (via `%d`/`strconv.Itoa`), which
  can never contain a comma; `opts.ExtraArgs` is documented and intended to
  be appended verbatim (an escape hatch callers already control the escaping
  for, e.g. `ParseExtraArgsEnv`'s own test fixtures). None of these needed a
  change.
- Added `TestArgsDoublesLiteralCommasInImagePathForDriveFile`
  (`internal/qemurun/qemurun_test.go`), asserting the `-drive` value for a
  comma-bearing image path comes out with the comma doubled.

Gates run: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty),
`golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...` — all clean.
