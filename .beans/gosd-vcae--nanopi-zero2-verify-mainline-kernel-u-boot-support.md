---
# gosd-vcae
title: 'NanoPi Zero2: verify mainline kernel + U-Boot support for RK3528'
status: completed
type: task
priority: low
created_at: 2026-07-05T05:34:02Z
updated_at: 2026-07-06T02:23:00Z
parent: gosd-cwjf
---

Research task, gates the whole epic. Answer with source links (kernel.org / U-Boot git, not vendor wikis):
- [x] Does a mainline DTS exist for NanoPi Zero2 (rk3528-nanopi-zero2.dts or similar)? Since which kernel tag? If board DT is absent but rk3528.dtsi exists, list what a board DT would need (upstream it or wait — state recommendation per the mainline-only policy)
- [x] Mainline U-Boot: is there an RK3528 board family defconfig usable for this board? Which rkbin blobs (DDR init, BL31) does RK3528 need — exact paths + license check
- [x] GbE: which MAC/PHY (stmmac + which PHY driver)? USB gadget: which controller (dwc2/dwc3) on the USB-C device port, and is it usable as a peripheral in mainline?
- [x] Debug UART: which UART + baud (FriendlyElec convention is usually 1500000n8 — confirm), and where the pins are on this board
- [x] Deliver: findings appended here + a go/no-go recommendation; if no-go, set this epic priority to deferred with a "recheck at kernel vX.Y" note. RESULT: GO — see Recommendation section below.

## Acceptance
Every claim source-linked; a clear go/no-go on starting the build tasks.


## Findings (2026-07-06)

### 1. Mainline Linux DT — YES, exists

`arch/arm64/boot/dts/rockchip/rk3528-nanopi-zero2.dts` is in mainline Linux.

- Added: commit b944112ab "arm64: dts: rockchip: Add FriendlyElec NanoPi Zero2" — first
  contained in **v6.18-rc1** (verified via GitHub compare API ancestry against
  torvalds/linux: v6.17 diverged/does-not-contain, v6.18-rc1 contains it). Source:
  https://github.com/torvalds/linux/commits/master/arch/arm64/boot/dts/rockchip/rk3528-nanopi-zero2.dts
  Board profile: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/arch/arm64/boot/dts/rockchip/rk3528-nanopi-zero2.dts
- Follow-up commit ff660109f "arm64: dts: rockchip: Enable USB 2.0 ports on NanoPi Zero2"
  (2026-05-29) turns on `usb_host0_ehci`/`ohci`/`xhci` + `usb2phy*` (status was
  `disabled` before). This is **not yet in a numbered release** — confirmed absent from
  v6.18 and v6.19 (both "behind" the commit), present on Linus's `master` (current dev,
  heading to v6.20). Until a release/backport lands, we'd carry this ~15-line DT fragment
  ourselves (trivial — same shape as our existing per-board kernel fragments).
- `rk3528.dtsi` (SoC-level) already wires: GbE (`gmac0`/`gmac1`, `rockchip,rk3528-gmac`
  + `snps,dwmac-4.20a` — standard stmmac/dwmac-rk driver), eMMC (`sdhci`,
  `rockchip,rk3528-dwcmshc`/`rockchip,rk3588-dwcmshc` compatible — dwcmshc driver),
  SD (`sdmmc`, `rockchip,rk3528-dw-mshc`-style, dw_mmc core), USB3-capable DWC3 OTG
  controller (`usb_host0_xhci`, `rockchip,rk3528-dwc3`/`snps,dwc3`), and UART0
  (`snps,dw-apb-uart`, well-supported 8250/dw-apb-uart). None of this needed writing —
  it's all upstream already. Board DT is a "small addition" exactly as CLAUDE.md's
  mainline-only policy hopes for, not a case of missing SoC support.
- No pending/blocking patch series found on lore for this board — support is already
  merged, not in flight.

### 2. Mainline U-Boot — YES, dedicated defconfig exists

- `configs/nanopi-zero2-rk3528_defconfig` — added by commit ebf46b588 "board: rockchip:
  Add FriendlyElec NanoPi Zero2" (2026-01-10). First contained in **v2026.07-rc1**
  (v2026.04 diverges/does-not-contain; confirmed via GitHub compare API against
  u-boot/u-boot). v2026.07 is imminent/current (rc5 tagged as of today). Source:
  https://github.com/u-boot/u-boot/commits/master/configs/nanopi-zero2-rk3528_defconfig
- `CONFIG_DEFAULT_DEVICE_TREE="rockchip/rk3528-nanopi-zero2"` — U-Boot vendors Linux's DT
  via `dts/upstream` (synced copy); `dts/upstream/src/arm64/rockchip/rk3528-nanopi-zero2.dts`
  is present there too. No DT duplication/fork needed.
- The defconfig already turns on `CONFIG_USB_GADGET=y`, `CONFIG_USB_DWC3_GENERIC=y`,
  `CONFIG_USB_FUNCTION_ROCKUSB=y` — i.e. Rockusb/mass-storage gadget mode on the USB-C
  port is already a first-class U-Boot use case for this exact board upstream.
- rkbin blobs needed (per `doc/board/rockchip/rockchip.rst`'s rk3528 build recipe, which
  names `BL31`/`ROCKCHIP_TPL` env vars pointing at rkbin files — same pattern this repo
  already uses for the Radxa Zero 3E, see `build/boards/radxa-zero-3e/manifest.json`):
  - DDR init (TPL): `bin/rk35/rk3528_ddr_1056MHz_v1.13.bin` (generic) — a
    `..._LP4_4X_eyescan_v1.13.bin` variant also exists in rkbin for LPDDR4X-specific
    tuning (this board's RAM). Confirm which one the board profile should pin during
    the build task; either is present in the same rkbin repo/commit already pinned
    elsewhere in this project.
  - BL31 (ATF/TF-A EL3 firmware): `bin/rk35/rk3528_bl31_v1.21.elf`.
  - Optional BL32: `bin/rk35/rk3528_bl32_v1.06.bin` (RKTRUST ini references it; not
    required for a headless board with no TEE use case).
  - Source: https://github.com/rockchip-linux/rkbin (`RKBOOT/RK3528MINIALL.ini`,
    `RKTRUST/RK3528TRUST.ini`, `bin/rk35/`).
  - License: same rkbin `LICENSE` already vetted for Radxa Zero 3E — a Rockchip
    redistribution grant ("non-exclusive license to use, copy, distribute... and to
    modify... and sublicense, distribute such modifications"), which is what our
    manifest.json pattern already relies on to justify producing/distributing
    idbloader.img/u-boot.itb. No new license category to evaluate.

### 3. GbE MAC/PHY + USB-C device port controller

- GbE: SoC MAC is Synopsys DesignWare (`rockchip,rk3528-gmac` + `snps,dwmac-4.20a`,
  i.e. the `stmmac`/`dwmac-rk` driver family, same family already used for Radxa Zero 3E
  per bean gosd-c7tk). Board wires `gmac1` in RGMII mode (`phy-mode = "rgmii-id"`) via
  `mdio1`, PHY node uses generic `ethernet-phy-ieee802.3-c22` (autodetected at runtime,
  not named in DT — same pattern as Radxa Zero 3E). The physical chip is a
  **Realtek RTL8211F** (confirmed via FriendlyElec's own wiki/firmware changelog and
  corroborated by third-party coverage — vendor wiki 403s naive fetchers directly, used
  the Wayback Machine cache: https://web.archive.org/web/2026/https://wiki.friendlyelec.com/wiki/index.php/NanoPi_Zero2).
  `CONFIG_REALTEK_PHY` (kernel) is the driver — already a requirement on our Radxa Zero 3E
  kernel fragment, so no new PHY driver to add.
- USB-C device port: backed by the SoC's `usb_host0_xhci` node — a genuine
  **Synopsys DesignWare USB3 controller (dwc3)**, `compatible = "rockchip,rk3528-dwc3",
  "snps,dwc3"`, SoC-default `dr_mode = "otg"`. The board only wires the USB2 PHY
  (`usb2phy_otg`) to it — no SuperSpeed combphy connection on this board — so it's
  USB 2.0 High-Speed only, but the controller itself is fully OTG/peripheral-capable
  (this is exactly what U-Boot's Rockusb gadget config on this defconfig already
  exercises). A `dr_mode = "peripheral"` DT override (small fragment) is what GoSD's
  gadget setup would need, same shape as the Pi Zero 2W/Radxa dwc2/dwc3 gadget work.

### 4. Debug UART + baud, pin location

- UART0 (`serial@ff9f0000`, `rockchip,rk3528-uart`/`snps,dw-apb-uart`) is `serial0` /
  `stdout-path = "serial0:1500000n8"` in the board DT — **1500000 baud, 8n1**, confirming
  the FriendlyElec convention. U-Boot's defconfig matches:
  `CONFIG_BAUDRATE=1500000`, `CONFIG_DEBUG_UART_BASE=0xFF9F0000` (uart0).
- Pin mux: `uart0m0_xfer` = GPIO4_C7 (RX) / GPIO4_D0 (TX), pull-up
  (`rk3528-pinctrl.dtsi`).
- Physical location: **not** on the 30-pin FPC GPIO connector — FriendlyElec breaks debug
  UART out on a separate **8-pin 2.54mm header**: pin 3 = UART2DBG_TX, pin 5 =
  UART2DBG_RX, pins 1/6/8 = GND, pins 2/4 = VCC5V0_SYS, pin 7 = VCC_3V3. 3.3V level,
  1500000bps (matches DT). Source (vendor wiki, fetched via Wayback Machine since the
  live site 403s naive fetchers):
  https://web.archive.org/web/2026/https://wiki.friendlyelec.com/wiki/index.php/NanoPi_Zero2

## Recommendation: GO

All four gating questions resolve favorably and with primary sources:

1. Mainline Linux board DT exists (since v6.18-rc1); only a small, already-written
   upstream DT fragment (USB2 enable, currently unreleased on master) might need
   temporary local carrying, per CLAUDE.md's mainline-only policy this is a normal
   "small addition," not a BSP dependency.
2. Mainline U-Boot has a dedicated `nanopi-zero2-rk3528_defconfig` (landing in the
   imminent v2026.07 release) with USB gadget/Rockusb already enabled.
3. GbE (dwmac/stmmac + RTL8211F, same driver family as Radxa Zero 3E) and the USB-C
   device port (dwc3, OTG-capable) are both mainline-supported and gadget-capable.
4. Debug UART is confirmed 1500000n8 on UART0, on a dedicated 8-pin 2.54mm header
   (not the 30-pin FPC), pins documented above.

No vendor BSP kernel is required at any point in this chain — this board can be built
the same way as Radxa Zero 3E (mainline kernel + mainline U-Boot + rkbin blobs via the
existing no-rehosting pinned-blob pattern). Epic gosd-cwjf's "wait for mainline" gate is
satisfied; the board build tasks can proceed after v0.2 ships as planned. Epic priority
left as-is (no defer needed).
