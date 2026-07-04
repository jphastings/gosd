---
# gosd-gbsz
title: 'Radxa Zero 3E board profile: bootloader raw-writes + extlinux.conf'
status: completed
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-04T12:16:19Z
parent: gosd-v370
blocked_by:
    - gosd-d458
    - gosd-cvzt
---

Fill in the radxa-zero-3e board profile so the generic image writer produces a Rockchip-bootable image.

RawWrites (locked offsets): idbloader.img at byte offset 32768 (LBA 64), u-boot.itb at byte offset 8388608 (LBA 16384). Both land in the unpartitioned gap before the 16MiB partition start — the image writer already enforces non-overlap; add a size assertion that u-boot.itb fits before 16MiB.

BootFiles: `Image`, `rk3566-radxa-zero-3e.dtb`, `initramfs.cpio.zst`, and `extlinux/extlinux.conf` rendered from a go:embed template (locked content):
```
default gosd
timeout 0
label gosd
    kernel /Image
    fdt /rk3566-radxa-zero-3e.dtb
    initrd /initramfs.cpio.zst
    append console=ttyS2,1500000n8 quiet init=/init gosd.board=radxa-zero-3e
```
FirmwareFiles: empty map (no runtime-loaded firmware on this board in v0.1).

- [x] RawWrites + size guards
- [x] extlinux template + render test
- [x] Integration test: full build with fake artifacts, read image back, assert extlinux.conf content and raw bytes at both offsets

## Acceptance
`gosd build ./examples/hello --board=radxa-zero-3e ... -o x.img` passes the read-back integration test.

## Summary of Changes

Implemented the radxa-zero-3e Board profile (internal/boards/radxazero3e/board.go):

- Artifacts(): idbloader.img, u-boot.itb, Image, rk3566-radxa-zero-3e.dtb - all with no pinned URL yet (must come from --artifacts-dir, same as pi-zero-2w's kernel8.img), since they're built by build/boards/radxa-zero-3e/{uboot,kernel}/build.sh with no automatic-fetch mechanism into the CLI.
- RawWrites(): idbloader.img at offset 32768, u-boot.itb at offset 8388608, both read fully into memory so u-boot.itb's size can be checked against the 16MiB boot-partition start before internal/image's own overlap guard ever runs. Board.RawWrites has no error return, so a violation panics with an actionable message (same convention as Artifacts.MustOpen).
- BootFiles(): Image, rk3566-radxa-zero-3e.dtb, initramfs.cpio.zst, and extlinux/extlinux.conf rendered from a new go:embed text/template package (internal/boards/radxazero3e/templates) with the bean's exact locked content.
- FirmwareFiles(): empty map, per the locked v0.1 decision.

Extended cmd/gosd/build_integration_test.go with two new tests: a radxa-zero-3e fake-artifacts build that reads the finished image back and asserts extlinux.conf's exact content plus the raw bytes at both locked offsets, and a no---board test confirming both pi-zero-2w and radxa-zero-3e images are now produced. Added the four new fake artifact files to cmd/gosd/testdata/fake-artifacts (shared with the pi-zero-2w test).

No deviations from the bean's locked decisions.
