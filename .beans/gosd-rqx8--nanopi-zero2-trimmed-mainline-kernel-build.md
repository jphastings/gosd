---
# gosd-rqx8
title: 'NanoPi Zero2: trimmed mainline kernel build'
status: in-progress
type: task
priority: low
created_at: 2026-07-05T05:34:03Z
updated_at: 2026-07-06T05:50:23Z
parent: gosd-cwjf
blocked_by:
    - gosd-vcae
---

Mirror build/boards/radxa-zero-3e/kernel/: Dockerized build.sh from a pinned mainline tag, defconfig + committed fragment via merge_config, CONFIG_MODULES=n, everything =y: the same core/initramfs-zstd/vfat/net baseline as the other boards plus RK3528 SoC support, GbE (per research findings), USB gadget stack (per research: dwc2 or dwc3 + configfs functions), GPIO/I2C/SPI (rockchip drivers), serial console. Outputs Image + the board DTB to gitignored out/. Commit the generated full kernel.config with provenance header.

## Deviation from bean text (per CLAUDE.md: flag rather than silently diverge)
The bean text says "USB gadget stack (per research: dwc2 or dwc3 + configfs functions)". At the pinned kernel tag this is not usable: v6.18.37's rk3528.dtsi has no USB host/OTG controller DT node at all (verified directly against the tag's source — checked rk3528.dtsi and every RK3528 board file in-tree, plus phy-rockchip-inno-usb2.c's of_device_id table). This is consistent with gosd-vcae's archived findings: the dwc3 node (rockchip,rk3528-dwc3) and the board's USB2-enable commit (ff660109f) exist on Linus's master but are not in any numbered release yet (absent from v6.18 and v6.19). So the fragment neither requires nor asserts the USB gadget options at this tag; they'll become meaningful with a future fleet-wide kernel bump (the arm64 defconfig baseline already compiles the stack in regardless — see Config survival results below). GbE, storage (SD+eMMC), and serial console are fully supported and are what's implemented here, matching the epic's Ethernet-first scope.

## Todos
- [x] Mirror the radxa-zero-3e kernel pipeline: build.sh (Docker, debian:bookworm, aarch64-linux-gnu- cross), docker-build.sh, kernel-fragment.config, .gitignore, README.md under build/boards/nanopi-zero2/kernel/
- [x] Pin the same kernel tag as the fleet (v6.18.37) and verify rk3528-nanopi-zero2.dts exists at that tag before building (it does; upstream since v6.18-rc1)
- [x] Build for real: Image (68,262,400 bytes / ~65 MiB) + rk3528-nanopi-zero2.dtb (30,567 bytes) produced in gitignored out/
- [x] Commit the generated full kernel.config with provenance header
- [x] Assert key options survived olddefconfig (all did — see Config survival below)
- [x] Add nanopi-zero2-kernel job + packaging/provenance/release entries to .github/workflows/build-artifacts.yml (kernel only; U-Boot gated on gosd-f39b); actionlint clean
- [ ] Boot-test on hardware (blocked: no hardware; tracked by gosd-odp7 via gosd-wskc/gosd-f39b)

## Config survival results
Every fragment option survived `make olddefconfig` in the committed kernel.config: ARCH_ROCKCHIP, MMC_DW(+_ROCKCHIP), MMC_SDHCI_OF_DWCMSHC, STMMAC_ETH(+PLATFORM), DWMAC_ROCKCHIP, REALTEK_PHY, GPIO_ROCKCHIP, GPIO_CDEV, I2C_RK3X, SPI_ROCKCHIP, SERIAL_8250(+CONSOLE,+DW), the initramfs-zstd/vfat/net baseline, CONFIG_MODULES unset, RD_* alternatives unset, WLAN/CFG80211/SOUND/BT/DRM off, DEBUG_INFO_NONE, CC_OPTIMIZE_FOR_PERFORMANCE. None were dropped.

Finding worth recording: diffing the generated config against radxa-zero-3e's committed kernel.config, the ONLY option-line difference is CONFIG_MOTORCOMM_PHY (off here, on for radxa's YT8531 board variant). The arm64 defconfig baseline already enables the whole USB/dwc3/gadget-configfs stack on both boards — radxa's fragment USB block mostly restates defconfig. So this board's kernel still contains those USB symbols (=y, inert at runtime — no RK3528 controller DT node to bind); the fragment simply stops requiring or asserting them. Documented in the pipeline README's diff table.
