---
# gosd-o7jv
title: 'Cubie A5E: board profile (extlinux + u-boot-sunxi-with-spl raw write)'
status: completed
type: task
priority: normal
created_at: 2026-08-06T22:33:44Z
updated_at: 2026-08-06T22:52:40Z
parent: gosd-h1wv
blocked_by:
    - gosd-jpc8
---

Implement internal/boards.Board for the Cubie A5E in internal/boards/cubiea5e, mirroring rock4se's shape but with the sunxi boot chain: ONE RawWrite (u-boot-sunxi-with-spl.bin at the research-pinned offset, expected 8KiB) instead of Rockchip's idbloader+itb pair, plus kernel Image, sun55i-a527-cubie-a5e.dtb, initramfs and a rendered extlinux/extlinux.conf on the FAT boot partition.

Pinned values (gosd-jpc8): raw write u-boot-sunxi-with-spl.bin at OFFSET 8192 (8KiB, sector 16); artifacts u-boot-sunxi-with-spl.bin + Image + sun55i-a527-cubie-a5e.dtb; console ttyS0 @ 115200 default (--console-baud supported via extlinux console=); USB gadget SUPPORTED at the pinned artifacts (board DT sets usb_otg dr_mode="peripheral", MUSB mainline at fleet tag) — no boot-file change needed for --usb-gadget, mirror rock4se's ignore-the-flag comment pattern.

## Todos

- [x] internal/boards/cubiea5e package: board.go + templates (extlinux.conf) + behavioral tests mirroring rock4se's
- [x] Size guard: u-boot-sunxi-with-spl.bin write must end at or before the boot-partition start (same panic-with-actionable-message pattern as rock4se's maxUbootEndBytes)
- [x] UsbGadgetSupport / ConsoleBaudSupport per research findings (honest Reason strings if unsupported)
- [x] RegisterInternal in cmd/gosd/build.go (public flip happens in the activation bean, NOT here)
- [x] Reserve cubie-a5e in CLAUDE.md's Board IDs locked-decision list (epic's first PR)
- [x] COMPATIBILITY.md row(s) for cubie-a5e marked as in-progress/internal
- [ ] Quality gates + PR

## Summary of Changes

- Added `internal/boards/cubiea5e` (board.go, templates/ package + tests), mirroring rock4se's shape: Name "cubie-a5e", Arch arm64, three no-URL Artifacts (`u-boot-sunxi-with-spl.bin`, `Image`, `sun55i-a527-cubie-a5e.dtb`), BootFiles rendering extlinux/extlinux.conf (kernel Image + DTB + initramfs.cpio.zst + extlinux, console default ttyS0,115200 per gosd-jpc8), ONE RawWrite (u-boot-sunxi-with-spl.bin at offset 8192/8KiB) with a size guard mirroring rock4se's maxUbootEndBytes pattern (panics with an actionable message if the artifact would overrun the 16MiB boot-partition start), empty FirmwareFiles, and UsbGadgetSupport/ConsoleBaudSupport both Supported:true per gosd-jpc8's research (board DT already pins usb_otg dr_mode=peripheral, MUSB mainline).
- Registered the board via `boards.RegisterInternal` in `cmd/gosd/build.go` (internal-only: reachable only via explicit `--board=cubie-a5e`; no kernel/U-Boot artifacts exist yet, so it's excluded from the default build set, --help, and catalog generation, same as qemu-virt/pre-activation rock-4se).
- Reserved `cubie-a5e` in CLAUDE.md's Board IDs locked-decision list, noting epic gosd-h1wv, Allwinner A527, and internal-until-activation status.
- Added a Bring-up status row and a footnote to COMPATIBILITY.md documenting current honest scope (Ethernet one port yes, USB gadget yes pending hardware verify, WiFi/SPI/NVMe/eMMC out at the pinned mainline tag), and a `cubie-a5e` row to docs/board-build-tags.md.
- No cross-bean invariant-test coupling needed resolving: kernelspec_test.go's board-enumerating lists (`allBoardIDs`, the DTS-patch allowlist, the kernelspec-outputs-vs-Artifacts map) are hardcoded literals independent of `internal/boards` registration, and cubie-a5e has no kernelspec entry yet (that's bean gosd-axtv, sequenced after this one per the epic) — so `go test ./...` passes unmodified with this board only registered, not kernelspec-resolvable. This is the mirror image of rock-4se's history (kernelspec entry landed before board registration, needing a scaffolding note); here board registration lands first with no kernelspec entry, so no test references cubie-a5e yet and nothing needed updating.
- All quality gates green: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`.
