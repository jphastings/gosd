---
# gosd-je2r
title: 'ROCK 4SE: verify mainline kernel + U-Boot support for RK3399-T'
status: todo
type: task
created_at: 2026-07-13T12:18:03Z
updated_at: 2026-07-13T12:18:03Z
parent: gosd-cuym
---

Viability research (mirror of gosd-vcae). Largely pre-answered during planning — this bean records the findings with links and pins the remaining specifics. GO/NO-GO outcome recorded here; downstream beans consume the pins.

Known going in: `rk3399-rock-4se.dts` is in mainline at the fleet tag; `rock-4se-rk3399_defconfig` is in U-Boot; RK3399 needs no rkbin blobs (open TPL DRAM init, BL31 from mainline TF-A — TF-A's rk3399 platform additionally needs the Cortex-M0 toolchain `gcc-arm-none-eabi` for the PMU firmware).

## Todo

- [ ] Confirm `rk3399-rock-4se.dts` at v6.18.37: serial alias/console (expected `ttyS2,1500000n8`), SD controller node+driver (dw_mmc vs sdhci-of-arasan), GbE PHY (expected DWMAC_ROCKCHIP + Realtek), USB PHY nodes
- [ ] Verify `rock-4se-rk3399_defconfig` exists at U-Boot v2026.04 (gosd-f39b precedent: verify before build work)
- [ ] Pick + pin a TF-A release tag; confirm `make PLAT=rk3399 bl31` inputs (aarch64 + arm-none-eabi toolchains)
- [ ] Identify the OTG-capable USB port and its dwc3 `dr_mode` in the DTS — record whether A2 needs a dr_mode DTS patch (betamin uses USB mass-storage gadget mode for video transfer)
- [ ] Identify header I2C bus nodes (ROCK 4 header: i2c7 pins 3/5, i2c2 27/28, i2c6 29/31 — verify) and header SPI node for A2's DTS patches
- [ ] Record all findings + GO/NO-GO in this bean
