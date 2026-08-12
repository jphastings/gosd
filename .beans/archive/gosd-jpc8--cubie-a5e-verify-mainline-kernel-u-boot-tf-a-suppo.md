---
# gosd-jpc8
title: 'Cubie A5E: verify mainline kernel + U-Boot + TF-A support for the A527'
status: completed
type: task
priority: normal
created_at: 2026-08-06T22:33:20Z
updated_at: 2026-08-06T22:41:26Z
parent: gosd-h1wv
---

GO/NO-GO research gate for the Cubie A5E epic, mirroring gosd-je2r (ROCK 4SE) / gosd-vcae (NanoPi Zero2). Every finding needs a primary-source link (kernel.org/u-boot git, TF-A docs, linux-sunxi wiki) recorded in this bean.

Preliminary (unverified) picture from epic planning: Linux has arch/arm64/boot/dts/allwinner/sun55i-a527-cubie-a5e.dts with EMAC0 enabled since ~6.16 and LED/PHY-reset fixes in 6.18; U-Boot has radxa-a5e_defconfig (A523 series merged ~2025); TF-A has PLAT=sun55i_a523; sunxi SD boot loads SPL from offset 8KiB.

## Todos

- [x] Confirm sun55i-a527-cubie-a5e.dts (or successor name) exists in the stable tree AT THE FLEET TAG v6.18.37, and diff what peripherals its DT actually enables (MMC, EMAC0, second GMAC, USB, PMIC regulators)
- [x] Confirm radxa-a5e_defconfig exists at U-Boot v2026.04 (the tag rock-4se pins); note SPL DRAM init config and whether the build needs anything beyond BL31= (SCP firmware optional? confirm build proceeds without crust)
- [x] Confirm TF-A sun55i_a523 platform exists at v2.15.0 (the tag rock-4se pins) and builds bl31 with no extra firmware inputs
- [x] Pin the exact SD-card BootROM load offset(s) for the A523 family (8KiB primary; 256KiB fallback?) from U-Boot's sunxi board docs, and the output artifact name (u-boot-sunxi-with-spl.bin)
- [x] Serial console: which UART reaches the 40-pin header / dedicated debug pins, Linux device name (ttyS0?), default baud for extlinux console=
- [x] Kernel driver list for the fragment's RequiredY: pinctrl/GPIO (sun55i), MMC (sunxi-mmc), Ethernet (dwmac-sun8i + PHY driver — which PHY chip?), UART (8250_DW?), I2C (mv64xxx), SPI (sun6i), RTC, and critically the AXP717/AXP323 PMIC + regulator stack (SD/eMMC rails likely depend on it)
- [x] USB gadget viability at the fleet tag: which controller backs the Type-C OTG port (MUSB? dwc3?), is a peripheral/OTG mode usable mainline, and what dr_mode does the board DT pin
- [x] PCIe/NVMe (M.2): does mainline v6.18.37 support A523 PCIe at all; document the USB3/PCIe combo-PHY mux tradeoff; recommend in/out of stock scope
- [x] Second Ethernet (GMAC200): driver + DT status at the fleet tag; in/out of scope call
- [x] Record GO/NO-GO + all pins in a Findings section here

## Findings (2026-08-06): GO

All pins verified against primary sources (GitHub contents API at explicit refs; kernel stable tree via kernel.googlesource.com mirror).

### U-Boot — mainline, at the tag we already use, ZERO patches

- `configs/radxa-cubie-a5e_defconfig` exists at **v2026.04** (the exact tag rock-4se pins) and v2026.07. NOTE the name: the pending-series name `radxa-a5e_defconfig` from the April-2025 list postings was renamed on merge — searching the old name 404s and cost this research a false "not merged" scare.
- The board's DT `dts/upstream/src/arm64/allwinner/sun55i-a527-cubie-a5e.dts` is in-tree at v2026.04 (8265 bytes; `CONFIG_DEFAULT_DEVICE_TREE="allwinner/sun55i-a527-cubie-a5e"`).
- Defconfig carries the board-critical LPDDR4 DRAM tuning (`CONFIG_DRAM_SUNXI_*`), AXP717 PMIC SPL setup on R_I2C (`CONFIG_R_I2C_ENABLE`, addr 0x35 family), `CONFIG_SUN8I_EMAC` + `CONFIG_PHY_REALTEK`, EHCI/OHCI. `CONFIG_MACH_SUN55I_A523` selects the open-source DRAM init (`arch/arm/mach-sunxi/dram_sun55i_a523.c`) — no boot0/blob.
- Build needs `BL31=` (SUNXI_BL31_BASE=0x54000 for A523); SCP is NOT used on A523 (SUNXI_SCP_BASE defaults 0x0) — pass `SCP=/dev/null` to silence the warning.
- Output: single `u-boot-sunxi-with-spl.bin`; SD write offset **8KiB (bs=1k seek=8, sector 16)**, BootROM fallback location 128KiB also exists on all ARM64 SoCs (doc/board/allwinner/sunxi.rst at master). 8KiB sits in our MBR pre-partition gap — no internal/image changes.

### TF-A — the ONE non-mainline pin

- Mainline TF-A has **no** sun55i_a523 platform at any release tag (v2.15.0 is latest; master's plat/allwinner = common, sun50i_a133/a64/h6/h616/r329). readthedocs "latest" pages describing sun55i_a523 are from WIP-doc builds — do not trust them as merge status.
- Community-standard source: **jernejsk/arm-trusted-firmware branch `a523`**, HEAD `b5de74a685fb73b784e45bbbd18dd9a0c528d8b2` (2025-01-28), contains `plat/allwinner/sun55i_a523`; build `make PLAT=sun55i_a523 bl31`. (apritzel's older fork is marked obsolete.) Pin that commit in manifest.json — source-compiled BSD-3-Clause, still blob-free; precedent for pinning a downstream fork: the Pi boards' raspberrypi/linux commit pin. Revisit when TF-A mainlines the platform.

### Kernel — fleet tag v6.18.37 has the board fully enabled for our needs

`arch/arm64/boot/dts/allwinner/sun55i-a527-cubie-a5e.dts` at v6.18.37, compatible `"radxa,cubie-a5e", "allwinner,sun55i-a527"`. Enabled (status="okay"): mmc0 (SD, cd PF6, **vmmc = AXP717 cldo3 — PMIC drivers are a hard SD dependency**), gmac0 (rgmii-id, external PHY addr 1, reset PH8), uart0 (`stdout-path = "serial0:115200n8"`), usb_otg (**dr_mode = "peripheral"**), usbphy, ehci0/1 + ohci0/1, gpu, 2 gpio-leds (PL4 heartbeat green, PL5 blue).

Compatible→driver map (fallback compatibles verified in sun55i-a523.dtsi at the tag):
- mmc0 `allwinner,sun20i-d1-mmc` → CONFIG_MMC_SUNXI
- gmac0 `allwinner,sun50i-a64-emac` → CONFIG_STMMAC_ETH + CONFIG_DWMAC_SUN8I (+ CONFIG_REALTEK_PHY for the RTL8211F)
- uart0 `snps,dw-apb-uart` → CONFIG_SERIAL_8250_DW (same as the Rockchip fleet)
- i2c/r_i2c `allwinner,sun6i-a31-i2c` → CONFIG_I2C_MV64XXX; PMICs AXP717@0x34 + AXP323@0x36 on r_i2c0 → CONFIG_MFD_AXP20X_I2C + CONFIG_REGULATOR_AXP20X
- usb_otg `allwinner,sun8i-a33-musb` → CONFIG_USB_MUSB_SUNXI (+ gadget stack) — **mainline USB gadget is viable on the Type-C OTG port at the fleet tag**
- rtc `allwinner,sun50i-r329-rtc` → CONFIG_RTC_DRV_SUN6I
- pinctrl `allwinner,sun55i-a523-pinctrl` (+ -r-pinctrl) → the sun55i pinctrl Kconfig symbols (kernel bean: read exact names from the tree)

### Scope consequences

- **Console**: ttyS0 @ 115200 — extlinux `console=ttyS0,115200`. Default baud differs from the Rockchip boards' 1500000.
- **USB gadget: IN scope** (board DT already pins peripheral mode; no DTS patch needed) — better than expected.
- **NVMe/PCIe: OUT** — no PCIe node at the fleet tag (A523 PCIe not mainlined); combo-PHY question moot for now.
- **Second GbE (GMAC200): OUT** — gmac1 not enabled/driver not at fleet tag (landed later upstream).
- **WiFi/BT: OUT** — module driver not mainline.
- **Header SPI: OUT for now** — sun55i-a523.dtsi has NO spi nodes at v6.18.37, so a spidev enablement DTS patch has nothing to attach to; revisit on a fleet tag bump. Header I2C: controller nodes exist; kernel bean decides which bus + DTS patch per convention.
- No eMMC node enabled in the board DT at this tag (SD-only is fine — matches epic scope).
