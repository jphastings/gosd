---
# gosd-2v40
title: qemu-virt board profile + gosd-init virtio device support
status: completed
type: task
priority: normal
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T08:58:11Z
parent: gosd-c54j
blocked_by:
    - gosd-5wm0
---

Register board id qemu-virt (internal: EXCLUDED from the default no---board build set and from `gosd build` help examples — add an Internal marker to the boards registry). Artifacts: Image only. BootFiles: Image + initramfs.cpio.zst at the FAT root (no config.txt/extlinux — qemu boots -kernel/-initrd directly; the FAT partition still carries gosd.toml and receives cloud-init files, keeping provisioning testable). RawWrites: none. FirmwareFiles: empty.

gosd-init: add /dev/vda1 (boot) and /dev/vda2 (data) to the device-candidate lists alongside mmcblk — behind the same probe logic; no qemu-specific code paths.

- [x] Board profile + integration test (fake artifacts; assert default build does NOT include qemu-virt, explicit --board=qemu-virt does)
- [x] gosd-init candidate lists + tests
- [x] Update CLAUDE.md board IDs line to mention qemu-virt (internal)

## Summary of Changes

- `internal/boards`: added `RegisterInternal` alongside `Register`, backed by an `internalOnly` set (not a change to the `Board` interface itself, so pizero2w/radxazero3e are untouched). `All()`/`IDs()` skip internal-only boards; `Find()` still resolves them by name (so `--board=qemu-virt` works); new `IsInternal(id)` helper lets callers (catalog generation) filter explicitly-selected boards.
- `internal/boards/qemuvirt`: new Board implementation. Artifacts = `Image` only (no pinned URL, resolved via --artifacts-dir or the CI-built artifact release, same pattern as the other boards' kernels). BootFiles = `Image` + `initramfs.cpio.zst` only — no config.txt/extlinux, since qemu boots via -kernel/-initrd. gosd.toml still lands at the FAT root because the pipeline adds it for every board. RawWrites = nil, FirmwareFiles = empty (virtio uses in-kernel drivers).
- `cmd/gosd/build.go`: registers qemu-virt via `boards.RegisterInternal`; `writeCatalog` now filters internal boards out of its image list before writing anything — a build selecting only internal boards with `--catalog` is a silent no-op (prints a note via `cmd.PrintErrln`, still succeeds) rather than an error, since `--catalog` on any normal public-board build is unaffected either way.
- `cmd/gosd-init/main.go`: appended `/dev/vda1`/`/dev/vda2` to `bootDevices`/`dataDevices`, after the mmcblk entries, with no qemu-specific branching — same probe loop.
- `cmd/gosd-init/internal/boot/mounts.go`: reworded `MountDataPartition`'s fast-ENOENT doc comment, which previously said "the same card"/"the card's partition table" — no longer accurate once a virtio-blk disk is a candidate alongside SD/eMMC. Logic is unchanged; the reasoning (whichever device the boot mount actually succeeded from is the one already known-scanned) still holds candidate-by-candidate as the list grows.
- `CLAUDE.md`: Board IDs line now mentions qemu-virt as internal-only, cross-referencing the existing "qemu-virt board" locked decision.
- Tests: `internal/boards/qemuvirt/board_test.go` (Name/Artifacts/BootFiles shape incl. absence of config.txt/extlinux/BootFiles requires initramfs/RawWrites empty/FirmwareFiles empty), `internal/boards/boards_test.go` (RegisterInternal findable-but-excluded, shared namespace with Register), `cmd/gosd/build_integration_test.go` (explicit --board=qemu-virt produces Image+initramfs+gosd.toml with no bootloader config files; default build asserted to produce exactly 2 images and no hello-qemu-virt.img; --catalog on a qemu-virt-only build writes no os_list.json but still builds the image), `cmd/gosd-init/main_test.go` (candidate-list contents/order), `cmd/gosd-init/internal/boot/mounts_test.go` (fast-ENOENT path with a 3-candidate list).

Deviation from a strict reading of the bean: the bean's suggested marker name was "Internal() bool"; went with a separate `RegisterInternal` registration function instead (bean explicitly allowed this: "a separate registration list is fine too") so the `Board` interface and existing board packages needed zero changes.

Booting the image under qemu itself is out of scope here — that's gosd-27lz.
