---
# gosd-8rw2
title: 'diskfmt: Inspect reports FAT12/FAT16 volumes as FAT32 — refusal messages name a filesystem that isn''t there'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:59:13Z
updated_at: 2026-08-07T16:05:32Z
---

Found by review sweep `gosd-fuxs` (storage area), verified.

diskfmt.go:156-165: isFAT accepts TypeFat32|TypeFat16|TypeFat12 but the
Contents returned always says FS: FAT32. A FAT16 stick full of user data
gets refused with "already holds FAT32 labelled ..." — the app author's
only diagnostic, and it lies. Also makes Contents.FS useless for any
future width-sensitive decision.

**Fix:** either add FAT16/FAT12 FS values (all MountType "vfat") or
report the honest string "FAT". Behavioral test: Inspect of a FAT16 image
reports something a user would recognise.

## Todos

- [x] Add FAT16/FAT12 as their own `diskfmt.FS` values (both `MountType()` "vfat"), per the bean's first option
- [x] `inspectFAT`/`fatWidth` report the actual go-diskfs FAT width instead of hardcoding FAT32
- [x] Behavioral tests: Inspect of a real FAT16 fixture and a real FAT12 fixture each report their own honest FS/String()/MountType()
- [x] Confirmed callers (`internal/blockmount`, `cmd/gosd-init/internal/dataexpand`) need no changes — they already print `Contents.FS` through its `String()` Stringer
- [x] Quality gates + PR

## Summary of Changes

- `internal/diskfmt/diskfmt.go`: added `FAT16`/`FAT12` `FS` constants (both `MountType()` "vfat", same as FAT32); `inspectFAT` now maps the go-diskfs `filesystem.Type` it actually found (via new `fatWidth` helper, replacing the old boolean `isFAT`) to the matching `FS` value instead of hardcoding `FAT32`. `Format` is untouched — GoSD still only ever writes FAT32; this only changes how a *foreign* FAT16/FAT12 volume already on the device is classified and named.
- `internal/diskfmt/diskfmt_test.go`: added `TestInspectReportsFAT16Label` / `TestInspectReportsFAT12Label`, using a new `fatFixture` helper that calls go-diskfs's own `disk.Disk.CreateFilesystem` directly (there's no `FormatFAT16`/`FormatFAT12` in diskfmt to build the fixture with — GoSD never writes those widths) — the same public API the package's existing tests already use to *verify* FAT32 output, just used here to *construct* the input. Each test also asserts `String()` ("FAT16"/"FAT12") and `MountType()` ("vfat").
- No caller changes needed: `internal/blockmount`'s `describe` and `cmd/gosd-init/internal/dataexpand`'s `describeContents` both already print `Contents.FS` through its `String()` Stringer, so they immediately start naming FAT16/FAT12 honestly. Read-only-checked both packages' error-message call sites and their tests (which construct `diskfmt.Contents{FS: diskfmt.FAT32, ...}` fakes directly, bypassing `Inspect`) to confirm no test depended on the old always-FAT32 behavior.
- Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...` all green. Note: this machine had many sibling worktree agents running concurrent `go test`/`golangci-lint` at the same time, which twice made `cmd/gosd`'s tests hang on a contended shared Go build cache (a subprocess `go list` call inside `internal/build.requireMainPackage` never returned) until re-run with an isolated `GOCACHE`/`GOLANGCI_LINT_CACHE` per CLAUDE.md's documented remedy — not a regression from this change (confirmed: `internal/diskfmt` itself passed cleanly on every attempt, including the ones where `cmd/gosd` hung).
