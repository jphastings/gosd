---
# gosd-gbsz
title: 'Radxa Zero 3E board profile: bootloader raw-writes + extlinux.conf'
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-02T21:10:20Z
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

- [ ] RawWrites + size guards
- [ ] extlinux template + render test
- [ ] Integration test: full build with fake artifacts, read image back, assert extlinux.conf content and raw bytes at both offsets

## Acceptance
`gosd build ./examples/hello --board=radxa-zero-3e ... -o x.img` passes the read-back integration test.
