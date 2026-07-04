---
# gosd-c7tk
title: 'Radxa Zero 3E kernel: trimmed mainline arm64 build'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-03T17:52:48Z
parent: gosd-v370
---

Trimmed, module-free mainline kernel for the RK3566 Radxa Zero 3E.

Source (locked): mainline stable, latest LTS (>= 6.12), pinned by tag in build.sh. Start from arm64 `defconfig`, trim hard. Deliverables in `build/boards/radxa-zero-3e/kernel/`: `kernel.config`, Dockerized `build.sh`, outputs `Image` and `rk3566-radxa-zero-3e.dtb` (in-tree DT — confirm exact filename in arch/arm64/boot/dts/rockchip/).

Config requirements (all =y, CONFIG_MODULES=n), beyond the same core/initramfs/vfat/net baseline as the Pi task:
- SoC: CONFIG_ARCH_ROCKCHIP + required clk/pinctrl/pmdomain defaults that defconfig brings
- Storage: CONFIG_MMC_DW + CONFIG_MMC_DW_ROCKCHIP (SD), CONFIG_MMC_SDHCI_OF_DWCMSHC (harmless, covers eMMC variants)
- Ethernet: CONFIG_STMMAC_ETH + CONFIG_DWMAC_ROCKCHIP, CONFIG_REALTEK_PHY (board PHY is RTL8211F — verify against the DT and note here), CONFIG_MOTORCOMM_PHY too if DT suggests YT8531 on any revision
- USB: CONFIG_USB_DWC3 (+ dual-role), CONFIG_PHY_ROCKCHIP_INNO_USB2, CONFIG_PHY_ROCKCHIP_NANENG_COMBPHY (USB3), gadget configfs stack same as Pi task
- Peripherals: CONFIG_GPIO_ROCKCHIP, CONFIG_I2C_RK3X, CONFIG_SPI_ROCKCHIP, CONFIG_SERIAL_8250_DW (console ttyS2)
- No WiFi drivers at all (board has none); no sound/DRM/BT; disable debug info

- [ ] build.sh + kernel.config committed, pinned tag in header
- [ ] DT filename + PHY driver verified against source, findings noted here
- [ ] Boot-tested via serial with the bring-up task

## Acceptance
Clean build outputs Image + dtb; CONFIG_MODULES=n; boots to gosd-init with eth0 present.

## Verification findings (2026-07-03)

- **DT filename**: confirmed exact — `arch/arm64/boot/dts/rockchip/rk3566-radxa-zero-3e.dts` exists in-tree at the pinned tag (includes shared `rk3566-radxa-zero-3.dtsi`), matching the bean's guess exactly.
- **Ethernet PHY**: the DT's `&mdio1/ethernet-phy@1` node uses the generic `compatible = "ethernet-phy-ieee802.3-c22"` (PHY model is autodetected at runtime via the MDIO ID registers, not named in the DT). The physical chip is Radxa's documented RTL8211F-CG gigabit transceiver, matched at runtime by drivers/net/phy/realtek/ (CONFIG_REALTEK_PHY) — confirms the bean's expectation.
- **Kconfig symbol correction**: the bean names `CONFIG_PHY_ROCKCHIP_NANENG_COMBPHY`; verified against drivers/phy/rockchip/Kconfig in the pinned tree — the actual mainline symbol is `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY` (COMBO_PHY, not COMBPHY). Used the correct symbol in kernel-fragment.config.
- **Pinned source**: mainline stable "longterm" tag `v6.18.37` (kernel.org releases.json moniker "longterm", satisfies >= 6.12).
