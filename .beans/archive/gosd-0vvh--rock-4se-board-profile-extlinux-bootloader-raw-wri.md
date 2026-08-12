---
# gosd-0vvh
title: 'ROCK 4SE: board profile (extlinux + bootloader raw-writes)'
status: completed
type: task
priority: normal
created_at: 2026-07-13T12:41:54Z
updated_at: 2026-07-16T22:31:06Z
parent: gosd-cuym
blocked_by:
    - gosd-iosp
    - gosd-dtpo
---

New `internal/boards/rock4se/` mirroring `internal/boards/radxazero3e/` (board.go + templates/): Artifacts = idbloader.img, u-boot.itb, Image, rk3399-rock-4se.dtb (URL-less, artifact-release-resolved); RawWrites at 32768 / 8388608 with the ≤16MiB u-boot.itb guard; extlinux template with `console=ttyS2,1500000n8 quiet init=/init gosd.board=rock-4se`; FirmwareFiles empty (no WiFi in scope).

## Locked decisions

- Register via `boards.RegisterInternal` in `cmd/gosd/build.go` until the artifacts release exists (gosd-wskc precedent — public registration would 404 on artifact fetch); flip-to-Register condition commented, done in the activation bean.
- No artifacts.Version / catalog / COMPATIBILITY changes here.
- `TestKernelSpecOutputsMatchBoardArtifacts` must pass against A2's kernelspec entry.

## Todo

- [x] internal/boards/rock4se/ (board.go, templates, board_test.go) — board.go + templates landed with gosd-iosp (PR #88: build-kernel resolves boards via the registry, so gosd-iosp needed them); board_test.go landed here.
- [x] RegisterInternal in cmd/gosd/build.go (landed with gosd-iosp, PR #88, same coupling)
- [x] Extend cmd/gosd/build_integration_test.go (fake artifacts incl. rk3399-rock-4se.dtb: raw writes, boot partition contents, exact extlinux.conf; kept excluded from default all-boards set until activation)
- [x] docs/board-build-tags.md entry

## Summary of Changes

Split across two PRs by the build-kernel↔registry coupling (see gosd-iosp):

**PR #88 (with gosd-iosp):** `internal/boards/rock4se/` board.go +
templates/ (extlinux.conf.tmpl, templates.go) + `RegisterInternal` in
cmd/gosd/build.go.

**This bean's own branch `bean/gosd-0vvh-rock4se-board-tests`:**
- `internal/boards/rock4se/board_test.go` — behavioral mirror of the
  radxazero3e suite: artifact names URL-less, BootFiles contents +
  initramfs requirement, extlinux `gosd.board=rock-4se`, RawWrites at
  32768/8388608 with content checks, oversized-u-boot 16MiB panic,
  empty FirmwareFiles, UsbGadget no-op.
- `cmd/gosd/build_integration_test.go` —
  `TestBuildProducesABootableImageForRock4SEFromFakeArtifacts` (network
  tripwire, raw-write readback at both offsets, boot-partition contents,
  byte-exact extlinux.conf) + new fake
  `cmd/gosd/testdata/fake-artifacts/rk3399-rock-4se.dtb`. Also updated
  `TestBuildWithNoBoardFlagBuildsAllBoards` to assert the absence of BOTH
  internal boards' images (its comment previously called qemu-virt "the
  only remaining internal-only board").
- `docs/board-build-tags.md` — `rock-4se` / `gosd_rock_4se` row + a
  prose note that the board is internal-only until the activation bean
  flips it public.
