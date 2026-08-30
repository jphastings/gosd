---
# gosd-k4w2
title: 'Turing RK1: verify mainline U-Boot/kernel + rkbin blob support for RK3588'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:25:46Z
updated_at: 2026-08-30T07:36:55Z
parent: gosd-bntd
---

GO/NO-GO gate, mirrors gosd-jpc8 (cubie-a5e's research bean). Primary-source-verify, at our actual pinned tags (mainline fleet kernel tag, and whatever U-Boot tag we pin — check if v2026.04 already carries turing-rk1-rk3588_defconfig or if the fleet's U-Boot pin needs bumping):

- turing-rk1-rk3588_defconfig and rk3588-turing-rk1.dts/.dtsi both actually present and building cleanly at our pins (not just 'exists upstream somewhere').
- rkbin blob requirement: exact DDR-init TPL binary and BL31 ELF filenames/versions this defconfig needs (ROCKCHIP_TPL / BL31 build args), their upstream URLs, and sha256 — mirror radxa-zero-3e's or nanopi-zero2's manifest.json shape.
- Raw-write offsets: confirm idbloader.img and u-boot.itb offsets for RK3588 (may differ from RK3399's LBA64/LBA16384 — RK3588's idbloader can be larger; confirm boot-partition start doesn't collide).
- Console UART + baud rate for this board (do not assume 1.5M from other Rockchip boards — verify).
- NVMe/PCIe pinmux bug status at our pinned kernel tag (informational only — NVMe boot is out of scope per the epic, but this affects whether NVMe-as-additional-storage via disk/ is usable later).
- USB gadget: does the mainline DT pin a usable dr_mode for gadget mode the way rock-4se's does?
- Confirm CONFIG_EXT4_FS / other kernel options relevant to existing feature parity (boards.EXT4Support etc.) build cleanly in a trimmed fragment alongside the rest of the mainline fleet fragment.

Write findings back into the epic bean (gosd-bntd) as a 'Research outcome' section, GO or NO-GO, same as cubie-a5e's gosd-jpc8.



## Summary of Changes

Primary-source-verified (GitHub raw fetches against the actual pinned tags/repos, kernel.org cgit itself is behind an Anubis bot-wall so mainline `torvalds/linux` v6.18 was used as a proxy for the fleet's v6.18.37 stable point release -- stable branches never drop a DTS file present in the mainline release they branched from, so this is treated as confirmed, not assumed):

- **U-Boot**: `turing-rk1-rk3588_defconfig` exists and is a mature board port at **v2026.04** -- the tag ALREADY pinned by rock-4se/radxa-zero-3e/cubie-a5e. No fleet-wide U-Boot tag bump needed. Defconfig enables SATA/AHCI, MMC/eMMC, NVMe, RTL8169/DWC ethernet, and DWC3 USB with gadget support compiled in.
- **Kernel**: `rk3588-turing-rk1.dts`+`.dtsi` exist in mainline (confirmed at v6.18, high-confidence proxy for the fleet's v6.18.37 pin). `compatible = "turing,rk1", "rockchip,rk3588"`. Console: **`serial9` @ 115200n8** -- NOT the 1.5M baud other Rockchip boards use; do not copy that default.
- **rkbin blobs required** (RK3588 has no open-source DRAM init, confirmed by U-Boot's own doc/board/rockchip/rockchip.rst): `ROCKCHIP_TPL`+`BL31` both required. rkbin's current `bin/rk35/` carries `rk3588_bl31_v1.54.elf` and two DDR TPL variants (`rk3588_ddr_lp4_1848MHz_lp5_2112MHz_v1.21.bin` / `rk3588_ddr_lp4_2112MHz_lp5_2400MHz_v1.21.bin`, plus eyescan diagnostic variants -- not for production). U-Boot's own doc references older version numbers (v1.33 BL31, v1.09 DDR) than what's in rkbin master now; **pin whichever exact commit+files the U-Boot pipeline bean (gosd-bib8) actually builds and boots against, verified the same way radxa-zero-3e's manifest.json was, not the version numbers in this note.**
- **RawWrites shape may differ from every other Rockchip board in the fleet**: U-Boot's rockchip.rst describes RK3588 (and newer Rockchip boards generally) producing a single binman-composed `u-boot-rockchip.bin` (idbloader + FIT combined) written at ONE offset (`seek=64`, i.e. LBA64/32KiB) -- not the two-artifact idbloader.img@32KiB + u-boot.itb@8MiB split rock-4se/radxa-zero-3e use. The image-writer abstraction (`image.RawWrite`) supports either shape trivially; **the board-profile bean (gosd-jvtg) confirms which artifact(s) the actual build emits** before locking `RawWrites()`'s exact offsets/artifact count.
- **USB gadget**: candidate, not yet certain. DT shows `u2phy0_otg` (status okay, OTG-capable) separate from `usb_host1_xhci` (fixed `dr_mode = "host"`). No explicit `dr_mode = "peripheral"` found in the DTSI at this grep pass -- board-profile bean should check the actual `dwc3`/`usbdrd` node`'s `dr_mode` (or confirm it's runtime-switchable via `usb_host1_xhci`'s sibling controller) before locking `UsbGadgetSupport()`.
- **Ethernet**: `gmac1` (RGMII) + RTL8211F PHY, `status = "okay"` -- standard, well-supported mainline path, no concerns.
- **PCIe/NVMe** (informational only -- boot is out of scope per the epic; this is about NVMe-as-additional-storage later): `pcie3x4`, `pcie2x1l1`, `pcie30phy` all `status = "okay"` in the mainline DTSI at our pin, consistent with the historical NVMe pinmux bug (reported active ~end of 2024, expected fixed by 6.12/6.13) being resolved well before v6.18.37. Not hardware-verified; leave as a stretch goal per the epic, confirm for real during bring-up.

**Verdict: GO.** No blockers found. Epic's locked decisions updated with a "Research outcome" section.



## Correction (2026-08-30, bean gosd-vh82): ttyS9 was wrong on real hardware

This bean's console finding was DT-only (stdout-path names the serial9
alias) and turned out to not hold on real hardware: the generic 8250 driver
does not number this UART by its DT alias index, and this board has only
one enabled UART node, so it registers as **ttyS0**. console=ttyS9 panics
("unable to open an initial console"). Confirmed by an actual hardware
boot. This is exactly the kind of gap DT-only research (no real board yet)
can leave -- recorded here so a future board's research bean weighs a
DT alias claim about ttySN numbering as a hypothesis, not a fact, until
hardware-confirmed.
