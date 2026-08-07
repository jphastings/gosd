---
# gosd-h1wv
title: 'Board support: Radxa Cubie A5E'
status: todo
type: epic
priority: normal
created_at: 2026-08-06T22:32:57Z
updated_at: 2026-08-06T22:42:43Z
---

Seventh supported board and the FIRST ALLWINNER board: Radxa Cubie A5E — Allwinner A527 (sun55iw3 die, shared with A523/T527/H728), 8x Cortex-A55 (4x2.0GHz + 4x1.42GHz), arm64. Board: LPDDR4x (1/2/4GB variants), microSD + optional onboard eMMC, M.2 Key-M (PCIe Gen2 x1 via combo PHY shared with USB3), 2x GbE, WiFi 6/BT 5.4 module, USB 3.0 Type-A host, USB 2.0 Type-C OTG (gadget candidate + power), 40-pin Pi-style header, HDMI 2.0. JP has the hardware (purchased 2026-08).

Decomposition mirrors the ROCK 4SE epic (gosd-cuym). New SoC family, but the expected delta is small: internal/image needs zero changes (the sunxi boot chain is one RawWrite into the existing pre-partition gap), and the kernel joins the existing mainline fleet tag.

Refs: https://docs.radxa.com/en/cubie/a5e , https://linux-sunxi.org/Radxa_Cubie_A5E , https://www.cnx-software.com/2025/01/04/radxa-cubie-a5e-allwinner-a527-t527-sbc-with-hdmi-2-0-dual-gbe-wifi-6-bluetooth-5-4/

## Locked decisions

- **Board ID: `cubie-a5e`** (build tag `gosd_cubie_a5e`). Follows the rock-4se precedent (vendor dropped where the product-line name is distinctive) and matches the mainline DTS name (sun55i-a527-cubie-a5e). Reserve in CLAUDE.md's Board IDs list in this epic's first PR.
- **Mainline-only**, same rule as NanoPi Zero2: no Allwinner BSP kernel, no Radxa vendor U-Boot. If mainline support is absent or immature at our pins, the epic WAITS.
- **Joins the mainline fleet kernel tag** (kernelspec's fleetKernelTag, currently v6.18.37) — first non-Rockchip member; update that constant's "Rockchip-family" comment wording when adding the spec.
- **Boot chain (new to gosd, zero internal/image changes expected):** sunxi BootROM → single `u-boot-sunxi-with-spl.bin` (SPL + FIT: BL31 + U-Boot proper + DTB) raw-written at 8KiB into the unpartitioned gap → extlinux. ONE RawWrite where Rockchip needs two. Exact BootROM offsets for A523 confirmed by the research bean before any profile code.
- **Blob-free** (like rock-4se, unlike the rkbin boards): DRAM init is open-source in mainline U-Boot's SPL; BL31 compiled from mainline TF-A `make PLAT=sun55i_a523`. manifest.json records TF-A source (repo/tag/peeled commit/BSD-3-Clause), no blob section.
- **Ethernet-first:** EMAC0 (dwmac-sun8i, enabled for this board in mainline since ~6.16) is the supported NIC. The second GbE port (new GMAC200 IP) is in scope ONLY if its driver + DT node exist at the fleet tag; otherwise document one-port support in COMPATIBILITY.md. Onboard WiFi/BT module: **out of scope** for this epic (driver expected non-mainline); follow-up bean if/when wanted.
- **NVMe/PCIe (M.2): stock-kernel candidate ONLY if mainline supports A523 PCIe at the fleet tag** (combo PHY is shared with USB3 — research bean documents the mux tradeoff); otherwise out of scope + COMPATIBILITY.md note.
- **SD boot only.** eMMC/SPI boot out of scope.
- **Peripheral enablement (header I2C/SPI):** kernel-build DTS patches, per the non-Pi convention — do not assume runtime overlays.
- Boot time: best effort; bring-up records a power-on→/app baseline.

## Sequencing

research (GO/NO-GO gate) → board profile (RegisterInternal — de-facto prereq of the kernel build, per CLAUDE.md) → U-Boot pipeline ∥ kernel build → artifacts release + activation → hardware bring-up.

## Research outcome (gosd-jpc8, completed 2026-08-06): GO

Corrections/refinements to the locked decisions above, from primary-source verification:

- U-Boot: mainline **v2026.04** (the tag rock-4se already pins) fully supports this board — `radxa-cubie-a5e_defconfig` (NOT the pending-series name radxa-a5e_defconfig) + the board DT in dts/upstream. Zero carried U-Boot patches.
- TF-A: mainline has NO sun55i_a523 platform at any release. BL31 builds from **jernejsk/arm-trusted-firmware branch `a523`**, pinned at commit b5de74a685fb (2025-01-28), `make PLAT=sun55i_a523 bl31`. Still source-compiled/blob-free; fork-pin precedent: the Pi boards' raspberrypi/linux commit pin. Revisit on TF-A mainlining. SCP not used on A523.
- Console: **ttyS0 @ 115200** (not the Rockchip 1.5M).
- USB gadget: **IN scope, mainline** — the board DT pins usb_otg dr_mode="peripheral" (MUSB, allwinner,sun8i-a33-musb) at the fleet tag.
- Confirmed OUT at fleet tag v6.18.37: PCIe/NVMe (no node), second GbE (GMAC200 later than v6.18), WiFi/BT (non-mainline driver), header SPI (dtsi has no spi nodes at all — nothing to patch spidev onto; revisit on fleet tag bump), eMMC (no node enabled in board DT).
- SD is powered by AXP717 cldo3: the AXP MFD/regulator + r_i2c drivers are hard kernel requirements or the card vanishes mid-boot.
