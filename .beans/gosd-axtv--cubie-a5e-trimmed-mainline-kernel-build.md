---
# gosd-axtv
title: 'Cubie A5E: trimmed mainline kernel build'
status: todo
type: task
priority: normal
created_at: 2026-08-06T22:34:12Z
updated_at: 2026-08-06T22:34:59Z
parent: gosd-h1wv
blocked_by:
    - gosd-jpc8
    - gosd-o7jv
---

Add the cubie-a5e KernelSpec (internal/kernelspec) + build/boards/cubie-a5e/kernel/{kernel-fragment.config,kernelassets.go,README.md}: mainline stable at the FLEET TAG (fleetKernelTag v6.18.37 — first non-Rockchip member; update the constant's "Rockchip-family" doc comment), arm64 defconfig + GoSD fragment, monolithic (ModulesDisabled), DTB target allwinner/sun55i-a527-cubie-a5e.dtb.

RequiredY comes from the research bean gosd-jpc8's driver findings (pinctrl sun55i, sunxi MMC, dwmac-sun8i + PHY, AXP717/AXP323 PMIC + regulators, I2C mv64xxx, SPI sun6i, UART, exFAT). Trim policy identical to the fleet: no DRM/sound/media; WiFi out.

Registration prereq (CLAUDE.md): `gosd build-kernel --board cubie-a5e` only resolves once the board profile bean's RegisterInternal has landed — stack accordingly.

## Todos

- [ ] kernelspec entry + fragment + embedded assets package
- [ ] Update the three board-enumerating lists in kernelspec_test.go (board count, Rockchip DTS-patch allowlist — note cubie-a5e is NOT Rockchip, check how the allowlist generalizes — and the outputs-vs-Artifacts map)
- [ ] Header I2C/SPI DTS patches under kernel/patches/ if the research bean says the board DT leaves them disabled (per-SoC convention; verify each applies against the pinned tag)
- [ ] Audit the resulting kernel.config for defconfig surprises (=m promotions, phantom drivers — the Pi-trap lesson generalizes: grep for gadget zoo/hwsim/console assumptions)
- [ ] Full gosd build-kernel run (backgrounded, colima up first), record kernel.config + Image/DTB sizes in kernel/README.md
- [ ] Quality gates + PR
