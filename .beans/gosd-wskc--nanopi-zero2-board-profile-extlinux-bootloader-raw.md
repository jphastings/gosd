---
# gosd-wskc
title: 'NanoPi Zero2: board profile (extlinux + bootloader raw-writes)'
status: in-progress
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-07T14:15:50Z
parent: gosd-cwjf
blocked_by:
    - gosd-f39b
    - gosd-rqx8
---

Mirror internal/boards/radxazero3e: register board ID nanopi-zero2; Artifacts() = idbloader.img, u-boot.itb, Image, <board>.dtb; RawWrites at byte offsets 32768 and 8388608 with the u-boot.itb size guard; BootFiles = kernel, dtb, initramfs, extlinux/extlinux.conf (console + baud per research findings, init=/init gosd.board=nanopi-zero2); FirmwareFiles empty. Extend the fake-artifacts integration tests; no---board builds must now emit three images. Also add the board to the artifact pipeline (build-artifacts.yml + manifest) and to CLAUDE.md board IDs (already reserved) + README/docs.

## Scope amendment (2026-07-06, JP: progress as far as possible pre-hardware)
Proceed NOW without waiting for U-Boot v2026.07: implement the full profile + fake-artifact integration tests, but register via RegisterInternal (like qemu-virt) so default builds/catalog exclude it — real artifact fetches would 404 on the U-Boot files until the artifact release includes them. Add a clearly-marked TODO + a checklist item here: flip to public Register + add artifacts-pipeline U-Boot entries when gosd-f39b completes. RawWrites offsets/extlinux content: same pattern as radxa (verify DT/console details from the gosd-vcae findings: rk3528-nanopi-zero2.dtb, UART0 1500000, ttyS0).
- [ ] Flip to public registration once U-Boot artifacts publish (gated on gosd-f39b)


## Summary of Changes (2026-07-07)
Implemented internal/boards/nanopizero2, mirroring internal/boards/radxazero3e:
Artifacts() = idbloader.img, u-boot.itb, Image, rk3528-nanopi-zero2.dtb (no
per-file pinned URL, same as radxa); RawWrites at offsets 32768/8388608 with
the same 16MiB u-boot.itb size guard; BootFiles renders extlinux/extlinux.conf
from a go:embed template (kernel /Image, fdt /rk3528-nanopi-zero2.dtb, initrd
/initramfs.cpio.zst, append console=ttyS0,1500000n8 quiet init=/init
gosd.board=nanopi-zero2); FirmwareFiles is empty (no runtime-loaded firmware
needed). BuildConfig.UsbGadget is ignored - no boot-time gadget change is
possible while this board has no USB controller in any numbered mainline
kernel (per gosd-cwjf's USB gate finding).

Console verification: fetched the mainline DTS at kernel tag v6.18.37
(gregkh/linux-stable, since torvalds/linux only carries release tags, not
stable point releases). rk3528-nanopi-zero2.dts's /aliases node has exactly
one serial alias, `serial0 = &uart0` (no other serialN aliases to shift
numbering); rk3528.dtsi's uart0 node is `compatible = "rockchip,rk3528-uart",
"snps,dw-apb-uart"` - the standard 8250-family driver, which enumerates as
/dev/ttySN (not the RK3288-era ttyFIQ fiq-debugger pseudo-console, which
doesn't exist on this SoC/board). serial0 -> uart0 with no other alias
therefore confirms console=ttyS0,1500000n8, matching stdout-path =
"serial0:1500000n8" in the board DT. Documented with source citations in
internal/boards/nanopizero2/templates/templates.go's doc comment.

Registered via boards.RegisterInternal (qemu-virt precedent) in
cmd/gosd/build.go, with a prominent comment marking the flip-to-Register
condition (gated on gosd-f39b publishing U-Boot artifacts). Extended
cmd/gosd/build_integration_test.go: a new fake-artifacts acceptance test for
--board=nanopi-zero2 (raw writes, boot partition contents, exact
extlinux.conf), plus updated the no---board-flag and --catalog exclusion
tests to assert nanopi-zero2 stays excluded alongside qemu-virt. Added
cmd/gosd/testdata/fake-artifacts/rk3528-nanopi-zero2.dtb.

Did not touch COMPATIBILITY.md, internal/artifacts.Version, or the
artifacts-pipeline board list beyond what gosd-rqx8 already added (kernel-only
job) - those are the flip-to-public PR's responsibility per this bean's scope
amendment.
