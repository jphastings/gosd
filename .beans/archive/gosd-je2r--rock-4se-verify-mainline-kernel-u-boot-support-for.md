---
# gosd-je2r
title: 'ROCK 4SE: verify mainline kernel + U-Boot support for RK3399-T'
status: completed
type: task
priority: normal
created_at: 2026-07-13T12:18:03Z
updated_at: 2026-07-14T11:59:48Z
parent: gosd-cuym
---

Viability research (mirror of gosd-vcae). Largely pre-answered during planning — this bean records the findings with links and pins the remaining specifics. GO/NO-GO outcome recorded here; downstream beans consume the pins.

Known going in: `rk3399-rock-4se.dts` is in mainline at the fleet tag; `rock-4se-rk3399_defconfig` is in U-Boot; RK3399 needs no rkbin blobs (open TPL DRAM init, BL31 from mainline TF-A — TF-A's rk3399 platform additionally needs the Cortex-M0 toolchain `gcc-arm-none-eabi` for the PMU firmware).

## Todo

- [x] Confirm `rk3399-rock-4se.dts` at v6.18.37: serial alias/console (expected `ttyS2,1500000n8`), SD controller node+driver (dw_mmc vs sdhci-of-arasan), GbE PHY (expected DWMAC_ROCKCHIP + Realtek), USB PHY nodes — CONFIRMED, see Findings #1
- [x] Verify `rock-4se-rk3399_defconfig` exists at U-Boot v2026.04 (gosd-f39b precedent: verify before build work) — CONFIRMED, see Findings #2
- [x] Pick + pin a TF-A release tag; confirm `make PLAT=rk3399 bl31` inputs (aarch64 + arm-none-eabi toolchains) — PINNED v2.15.0, see Findings #3
- [x] Identify the OTG-capable USB port and its dwc3 `dr_mode` in the DTS — record whether A2 needs a dr_mode DTS patch (betamin uses USB mass-storage gadget mode for video transfer) — CONFIRMED, A2 NEEDS A PATCH, see Findings #4
- [x] Identify header I2C bus nodes (ROCK 4 header: i2c7 pins 3/5, i2c2 27/28, i2c6 29/31 — verify) and header SPI node for A2's DTS patches — CONFIRMED, see Findings #5
- [x] Record all findings + GO/NO-GO in this bean — see Findings + GO/NO-GO sections below


## GO/NO-GO: GO

Every claim in the epic's locked-decisions section and this bean's "known
going in" note holds at the actual pinned tags (kernel v6.18.37, U-Boot
v2026.04). One adjustment surfaced: A2's scope must include an OTG
`dr_mode` DTS patch (not previously called out explicitly) and A2's
`RequiredY` list needs `CONFIG_PHY_ROCKCHIP_TYPEC` (RK3399's USB3 Type-C
PHY driver), not the RK3566-only combo-phy symbol radxa-zero-3e uses. No
blockers found.

## Findings

### 1. Kernel DTS at v6.18.37 — CONFIRMED (high confidence)

`arch/arm64/boot/dts/rockchip/rk3399-rock-4se.dts` exists at v6.18.37,
`#include`-ing `rk3399-t.dtsi` (T-variant OPP tables — lower clocks/
voltages than standard RK3399) and `rk3399-rock-pi-4.dtsi` (the shared
ROCK Pi 4 family board dtsi), which itself includes `rk3399-base.dtsi`.
Source: https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git/tree/arch/arm64/boot/dts/rockchip/rk3399-rock-4se.dts?h=v6.18.37 (fetched and read directly at the pinned tag).

- **Console**: `chosen { stdout-path = "serial2:1500000n8"; }` in
  `rk3399-rock-pi-4.dtsi` resolves via `rk3399-base.dtsi`'s
  `aliases { serial2 = &uart2; }` to `uart2: serial@ff1a0000`
  (`"rockchip,rk3399-uart", "snps,dw-apb-uart"` — driver 8250_dw,
  `CONFIG_SERIAL_8250_DW`, same symbol as radxa-zero-3e). Cross-validated
  against U-Boot's `rock-4se-rk3399_defconfig`:
  `CONFIG_DEBUG_UART_BASE=0xFF1A0000`, `CONFIG_BAUDRATE=1500000` — same
  physical register address and baud, independently confirming the same
  UART from two separate upstream projects. HIGH confidence on
  uart2/ff1a0000/1500000n8. The specific `/dev/ttyS2` device number
  (expected, by Rockchip 8250 alias-order convention) is MEDIUM
  confidence — not re-derived from an actual boot log.
- **SD controller**: `sdmmc: mmc@fe320000`, compatible
  `"rockchip,rk3399-dw-mshc", "rockchip,rk3288-dw-mshc"` — dw_mmc family
  (`CONFIG_MMC_DW` + `CONFIG_MMC_DW_ROCKCHIP`), the **same driver family
  as radxa-zero-3e** (not `sdhci-of-arasan` as the bean's todo worried).
  Enabled in `rk3399-rock-pi-4.dtsi` with `cd-gpios`, 4-bit bus, up to
  150MHz. HIGH confidence, direct from DTS text.
  - Side note: RK3399's **eMMC** controller (out of GoSD's SD-boot-only
    scope, recorded for completeness) is a *different* IP than
    radxa-zero-3e's: `sdhci: mmc@fe330000` compatible
    `"rockchip,rk3399-sdhci-5.1", "arasan,sdhci-5.1"` (Arasan SDHCI,
    `CONFIG_MMC_SDHCI_OF_ARASAN`) vs RK3566's DWC_MSHC eMMC
    (`CONFIG_MMC_SDHCI_OF_DWCMSHC`). Irrelevant unless a future bean adds
    eMMC support.
- **GbE**: `gmac: ethernet@fe300000`, compatible
  `"rockchip,rk3399-gmac"` (stmmac + `CONFIG_DWMAC_ROCKCHIP`, same
  symbol as radxa-zero-3e), `phy-mode = "rgmii"`, enabled via
  `rgmii_pins`. No explicit PHY-chip compatible node in the DTS (MDIO
  auto-probe is normal for this driver, so the DTS text alone can't name
  the chip). Corroborated by (a) U-Boot's `rock-4se-rk3399_defconfig`
  setting `CONFIG_PHY_REALTEK=y`, and (b) Radxa's own docs download page
  for the ROCK 4SE listing an "RTL8211E datasheet" alongside the board's
  schematic. HIGH confidence it's Realtek-family
  (`CONFIG_REALTEK_PHY`/`CONFIG_PHY_REALTEK`); MEDIUM-HIGH confidence on
  the exact RTL8211E part suffix (sourced from Radxa's docs site via
  search, not independently opened the schematic PDF).
- **USB PHYs**: `u2phy0`/`u2phy1` compatible
  `"rockchip,rk3399-usb2phy"` (`CONFIG_PHY_ROCKCHIP_INNO_USB2` — same
  driver as radxa-zero-3e); `tcphy0`/`tcphy1` compatible
  `"rockchip,rk3399-typec-phy"` (`CONFIG_PHY_ROCKCHIP_TYPEC` — **different
  from radxa-zero-3e's `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY`**, which is
  RK3566-only). A2's `RequiredY` list needs `CONFIG_PHY_ROCKCHIP_TYPEC`
  in place of the combo-phy symbol. HIGH confidence, from DTS compatible
  strings.
- Also present (out of this epic's scope, noted for the follow-up WiFi/BT
  bean): an `sdio0` node with a `brcm,bcm4329-fmac` WiFi child and a
  `uart0` `brcm,bcm4345c5` Bluetooth child — confirms the onboard
  WiFi/BT hardware has DT nodes ready at the fleet tag.

### 2. U-Boot defconfig at v2026.04 — CONFIRMED (high confidence)

`configs/rock-4se-rk3399_defconfig` exists at U-Boot tag v2026.04 — tag
verified via `git ls-remote --tags https://github.com/u-boot/u-boot.git`
(resolves to commit `32750a1d473aa4932de6303e62afc5306aee2b1f`, annotated
tag pointing at `88dc2788777babfd6322fa655df549a019aa1e69`). Defconfig
content fetched and read at that exact tag. Sets
`CONFIG_DEFAULT_DEVICE_TREE="rockchip/rk3399-rock-4se"`,
`CONFIG_TARGET_ROCKPI4_RK3399=y` (shares the ROCK Pi 4 board file),
`CONFIG_ROCKCHIP_SPI_IMAGE=y` + `CONFIG_TPL=y` + `CONFIG_SPL_SPI_LOAD=y`
— same idbloader+u-boot.itb SPI-image boot chain shape as radxa-zero-3e,
so `internal/image` needs zero changes (matches the epic's locked
decision). Also cross-listed by name in `doc/board/rockchip/rockchip.rst`
at the same tag: "Radxa ROCK 4SE (rock-4se-rk3399)". Source:
https://github.com/u-boot/u-boot/blob/v2026.04/configs/rock-4se-rk3399_defconfig

### 3. No rkbin blobs / TF-A from source — CONFIRMED (high confidence); TAG PINNED: v2.15.0

U-Boot's own `doc/board/rockchip/rockchip.rst` (read at v2026.04) gives
the canonical rk3399 build recipe:
```
export BL31=../trusted-firmware-a/build/rk3399/release/bl31/bl31.elf
make evb-rk3399_defconfig
make CROSS_COMPILE=aarch64-linux-gnu-
```
with **no** `ROCKCHIP_TPL` export — unlike the same doc's rk3308/rk3528/
rk3568 recipes, which explicitly `export ROCKCHIP_TPL=../rkbin/bin/...`.
This is a direct, dated, first-party confirmation that RK3399 needs no
DDR blob (open-source TPL DRAM init) while sibling Rockchip SoCs do.
HIGH confidence, straight from the pinned tag's own documentation.

TF-A tag pinned: **v2.15.0** (tagged 2026-05-28, commit
`9ad327a8d124ce82002614c23e33992d4de6f7cf`; verified via
`git ls-remote --tags` and a shallow tag checkout against
https://github.com/TrustedFirmware-A/trusted-firmware-a.git — the
project's own read-only GitHub mirror). Chosen over the prior stable
v2.14.0 (2025-11-24) as the more current point; TF-A ships roughly two
tagged releases a year (May/November) and v2.15.0 has had ~6 weeks to
surface build breaks as of this research.

`plat/rockchip/rk3399/platform.mk`, read at v2.15.0, confirms
`make PLAT=rk3399 bl31` builds `RK3399M0FW`/`RK3399M0PMUFW` from
`plat/rockchip/rk3399/drivers/m0` as a real build dependency of BL31 (the
PMU Cortex-M0 firmware is `.incbin`'d into `pmu_fw.S`, not optional).
`make_helpers/toolchains/rk3399-m0.mk` defaults that sub-build's
toolchain to `arm-none-eabi-gcc` (override via `M0_CROSS_COMPILE`) —
confirms the bean's claim that TF-A's rk3399 platform needs the
Cortex-M0 cross toolchain (`gcc-arm-none-eabi`) alongside
`aarch64-linux-gnu-`. HIGH confidence, from the pinned tag's own
Makefiles.

License: BSD-3-Clause, confirmed in `docs/license.rst` at v2.15.0 —
matches the epic's plan to record TF-A as "compiled-not-pinned" with
repo/tag/license in `manifest.json`, not a blob pin.

### 4. OTG/USB gadget dr_mode — CONFIRMED, A2 NEEDS A DTS PATCH (high confidence on the core finding; physical port mapping unresolved)

RK3399's base DTS (`rk3399-base.dtsi`) defines both dwc3 controllers
(`usbdrd_dwc3_0` @ fe800000, `usbdrd_dwc3_1` @ fe900000) with
`dr_mode = "otg"` by default. But `rk3399-rock-pi-4.dtsi` (included by
`rk3399-rock-4se.dts`) overrides **both** to `dr_mode = "host"`
explicitly:
```
&usbdrd_dwc3_0 { status = "okay"; dr_mode = "host"; };
&usbdrd_dwc3_1 { status = "okay"; dr_mode = "host"; };
```
`rk3399-rock-4se.dts` does not re-override either node, so it inherits
both as fixed host-mode. **At v6.18.37, neither USB port is in a
gadget-capable `dr_mode` out of the box** — confirmed directly from DTS
text, HIGH confidence.

Consequence for A2: a DTS patch is required (per-SoC convention — patch,
not runtime overlay, per this project's locked Rockchip convention,
since the pinned U-Boot lacks `OF_LIBFDT_OVERLAY`), setting one
`usbdrd_dwc3_N` node's `dr_mode` to `"peripheral"`. `"peripheral"` is
recommended over `"otg"`: mainline's shared rock-pi-4.dtsi has no
extcon/ID-pin glue wired to either port (the `vbus_typec` regulator is
defined but unused as a consumer anywhere in the dtsi), so there's no
OTG role-detection plumbing to preserve, and `"peripheral"` matches what
a fixed mass-storage-gadget device needs.

Radxa's own hardware docs (wiki.radxa.com/Rock4/4se) describe the
physical port layout as "USB 3.0 OTG x1 (hardware host/device switch) /
USB 3.0 HOST x1 (dedicated) / USB 2.0 HOST x2" — i.e. there is a
physical USB-ID-switch OTG port on the board, corroborating that gadget
mode is hardware-supported and just needs the DTS-side fix.

**Not resolved at DTS-text level**: which of `usbdrd_dwc3_0`/
`usbdrd_dwc3_1` (equivalently, `u2phy0`+`tcphy0` vs `u2phy1`+`tcphy1`) is
wired to the physical hardware-switch OTG port vs. the dedicated-host
port — the shared dtsi treats both symmetrically (same `vcc5v0_host`
supply, no distinguishing regulator/extcon reference). This needs a
schematic-level check (Radxa publishes a ROCK 4SE schematic PDF via
their docs download page) or a boot-time check (`dmesg`/`lsusb` while
toggling the physical switch) during A2 bring-up — **flagged for A2, not
resolved here**.

### 5. Header I2C/SPI — CONFIRMED (high confidence for bus identity + driver; pin-number mapping sourced from Radxa docs, not an opened schematic)

`i2c2`, `i2c6`, `i2c7` (compatible `"rockchip,rk3399-i2c"`, driver
`CONFIG_I2C_RK3X` — same Kconfig symbol as radxa-zero-3e) are left
`status = "disabled"` in `rk3399-base.dtsi` and are **not** touched by
`rk3399-rock-pi-4.dtsi` or `rk3399-rock-4se.dts` — still disabled at
v6.18.37, confirming they're free for GoSD's header-enablement DTS
patches (same convention as radxa-zero-3e's i2c3/spi3 patches). HIGH
confidence, DTS text.

Physical pin mapping (from Radxa's own docs/wiki, not visible in the DTS
text itself, which has no header-position comments): 40-pin header pins
3/5 = I2C7 (SDA7/SCL7), pins 27/28 = I2C2 (SDA2/SCL2), pins 29/31 = I2C6
(SCL6/SDA6) — matches the bean's expectation exactly. Sources:
https://docs.radxa.com/en/rock4/rock4ab-se/getting-started/interface-usage/gpio-headers
and https://wiki2.radxa.com/Rock4/hardware/gpio.

Header SPI: pins 19/21/23/24 = SPI1 (TXD/RXD/CLK/CS0) per the same Radxa
docs. `spi1` (compatible `"rockchip,rk3399-spi"`, driver
`CONFIG_SPI_ROCKCHIP` — same Kconfig symbol as radxa-zero-3e) is left
`status = "disabled"` in `rk3399-base.dtsi` and untouched by
`rk3399-rock-pi-4.dtsi`/`rk3399-rock-4se.dts` — confirmed free for a DTS
patch. MEDIUM-HIGH confidence overall: the bus-disabled fact is
DTS-text-confirmed (HIGH), the physical-pin-number mapping is
WebSearch-sourced from Radxa's own docs (not independently
cross-checked against a raw schematic PDF in this research pass) —
worth a quick re-check if a PN532 doesn't enumerate during A2 bring-up.

## Pinned decisions (for A2/A3 to consume)

- Fleet kernel tag: v6.18.37 (unchanged, fleet-wide)
- U-Boot tag: v2026.04 (unchanged, fleet-wide for Rockchip boards)
- TF-A tag: **v2.15.0** (new pin, specific to blob-free boards; record in
  `build/boards/rock-4se/manifest.json` alongside repo URL + BSD-3-Clause
  license note — TF-A is compiled from source, not pinned as a blob)
- Serial console: uart2 / 0xff1a0000, 1500000n8 (expected `/dev/ttyS2`,
  not independently confirmed against a live boot log)
- SD controller: `sdmmc` node, dw_mshc family — `CONFIG_MMC_DW` +
  `CONFIG_MMC_DW_ROCKCHIP` (same driver as radxa-zero-3e)
- GbE: `gmac` node, stmmac + `CONFIG_DWMAC_ROCKCHIP`; PHY Realtek
  RTL8211E family — `CONFIG_REALTEK_PHY` / `CONFIG_PHY_REALTEK`
- USB PHYs: `CONFIG_PHY_ROCKCHIP_INNO_USB2` (u2phy0/1) +
  **`CONFIG_PHY_ROCKCHIP_TYPEC`** (tcphy0/1 — NOT
  `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY`, that symbol is RK3566-only)
- OTG: A2 must add a DTS patch setting one `usbdrd_dwc3_N` node's
  `dr_mode` from the inherited `"host"` to `"peripheral"`. WHICH
  controller maps to the physical hardware-switch OTG port is unresolved
  here — needs a schematic or boot-time check in A2.
- Header I2C: i2c7 (pins 3/5), i2c2 (pins 27/28), i2c6 (pins 29/31) — all
  disabled upstream, need DTS patches; driver `CONFIG_I2C_RK3X`
- Header SPI: spi1 (pins 19/21/23/24, CS0 only) — disabled upstream,
  needs a DTS patch + spidev child node (`rohm,dh2228fv`, per the
  radxa-zero-3e precedent); driver `CONFIG_SPI_ROCKCHIP`

## Summary of Changes

Research-only bean: no code changed. Verified all five claims from the
epic's locked decisions and this bean's "known going in" note against the
actual pinned upstream tags (kernel v6.18.37, U-Boot v2026.04, and a
newly-picked TF-A v2.15.0), reading source files directly rather than
trusting memory. Recorded findings, confidence levels, source links, and
concrete pins above. GO — the epic can proceed to A1/A2, with two new
facts for A2 to carry: an OTG `dr_mode` DTS patch is required (which
physical port needs schematic-level confirmation), and A2's kernel
`RequiredY` list needs `CONFIG_PHY_ROCKCHIP_TYPEC` in place of
radxa-zero-3e's RK3566-only combo-phy symbol.
