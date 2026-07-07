---
# gosd-2j6z
title: 'Multi-arch builds: per-board GOARCH/GOARM'
status: completed
type: task
priority: normal
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T16:01:04Z
parent: gosd-ajpz
---

Keystone for pi-zero-w. Add an Arch concept to the Board interface/registry (e.g. Arch() returning {GOARCH, GOARM}; all existing boards return arm64). internal/build compiles the user app AND gosd-init once per distinct arch across the selected boards (cache per arch in the build dir; do not compile twice for two arm64 boards). The gosd-init module-cache path (internal/build/gosdinit.go) must pass the same env. CI smoke job adds a GOARM=6 cross-compile of examples/hello + cmd/gosd-init. Integration tests: fake board... keep it real — assert via the existing qemu-virt/pi builds that arm64 output is unchanged, plus a unit test that a GOARM=6 board yields an ARM 32-bit ELF (debug/elf: EM_ARM, ELFCLASS32).
- [x] Board interface arch + all boards updated
- [x] Per-arch compile pass with dedupe
- [x] GOARM=6 static ELF verified by test
- [x] CI smoke extended

## Summary of Changes

- Added `boards.Arch{GOARCH, GOARM}` (with a `Key()` method used as the
  compile-cache/dedupe key) and a `Board.Arch()` method; all four existing
  boards (pi-zero-2w, radxa-zero-3e, qemu-virt, plus the boards_test fake)
  return arm64.
- `internal/build.CrossCompile` and `CrossCompileGosdInit` now take a
  `boards.Arch` and build a shared `archEnv` (CGO_ENABLED=0, GOOS=linux,
  GOARCH, and GOARM when set) — both the local-checkout and module-cache
  gosd-init paths in gosdinit.go go through the same helper, so they cannot
  drift.
- `cmd/gosd build` now cross-compiles once per distinct arch across the
  selected boards via a new `compileForArchs` (cmd/gosd/archbuild.go),
  keyed by `Arch.Key()`, then maps each board to its arch's binaries.
  `cmd/gosd run` (qemu-virt only) passes its board's Arch straight through.
- Tests: dedupe is unit-tested directly against `compileForArchs` with
  invocation-counting fakes (3x arm64 -> 1 compile pass; +1 distinct arch ->
  2 passes); a GOARM=6 unit test in internal/build asserts a real static
  32-bit ARM ELF (ELFCLASS32, EM_ARM, no PT_INTERP) for both CrossCompile and
  CrossCompileGosdInit. The full existing integration suite (arm64 pi/radxa/
  qemu-virt image builds) passed unmodified, confirming no arm64 regression.
- CI: smoke-build job adds a `GOOS=linux GOARCH=arm GOARM=6 go build
  ./cmd/gosd-init ./examples/hello` step alongside the existing arm64 one.
- Did not touch COMPATIBILITY.md: no board or feature status changed, this
  is purely an internal build-pipeline change; pi-zero-w itself is gosd-et0q.
