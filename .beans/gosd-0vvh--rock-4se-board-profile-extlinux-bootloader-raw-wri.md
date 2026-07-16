---
# gosd-0vvh
title: 'ROCK 4SE: board profile (extlinux + bootloader raw-writes)'
status: in-progress
type: task
priority: normal
created_at: 2026-07-13T12:41:54Z
updated_at: 2026-07-16T16:05:32Z
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

- [ ] internal/boards/rock4se/ (board.go, templates, board_test.go) — board.go + templates landed on `bean/gosd-iosp-rock4se-kernel` (build-kernel resolves boards via the registry, so gosd-iosp needed them; see that bean). board_test.go still to do here.
- [x] RegisterInternal in cmd/gosd/build.go (landed on `bean/gosd-iosp-rock4se-kernel`, same coupling)
- [ ] Extend cmd/gosd/build_integration_test.go (fake artifacts incl. rk3399-rock-4se.dtb: raw writes, boot partition contents, exact extlinux.conf; keep excluded from default all-boards set until activation)
- [ ] docs/board-build-tags.md entry
