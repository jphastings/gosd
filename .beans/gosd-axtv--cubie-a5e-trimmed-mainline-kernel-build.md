---
# gosd-axtv
title: 'Cubie A5E: trimmed mainline kernel build'
status: in-progress
type: task
priority: normal
created_at: 2026-08-06T22:34:12Z
updated_at: 2026-08-07T11:34:54Z
parent: gosd-h1wv
blocked_by:
    - gosd-jpc8
    - gosd-o7jv
---

Add the cubie-a5e KernelSpec (internal/kernelspec) + build/boards/cubie-a5e/kernel/{kernel-fragment.config,kernelassets.go,README.md}: mainline stable at the FLEET TAG (fleetKernelTag v6.18.37 — first non-Rockchip member; update the constant's "Rockchip-family" doc comment), arm64 defconfig + GoSD fragment, monolithic (ModulesDisabled), DTB target allwinner/sun55i-a527-cubie-a5e.dtb.

RequiredY comes from the research bean gosd-jpc8's driver findings (pinctrl sun55i, sunxi MMC, dwmac-sun8i + PHY, AXP717/AXP323 PMIC + regulators, I2C mv64xxx, SPI sun6i, UART, exFAT). Trim policy identical to the fleet: no DRM/sound/media; WiFi out.

Registration prereq (CLAUDE.md): `gosd build-kernel --board cubie-a5e` only resolves once the board profile bean's RegisterInternal has landed — stack accordingly.

## Todos

- [x] kernelspec entry + fragment + embedded assets package
- [x] Update the three board-enumerating lists in kernelspec_test.go (board count, Rockchip DTS-patch allowlist — note cubie-a5e is NOT Rockchip, check how the allowlist generalizes — and the outputs-vs-Artifacts map)
- [ ] Header I2C/SPI DTS patches under kernel/patches/ if the research bean says the board DT leaves them disabled (per-SoC convention; verify each applies against the pinned tag)
- [ ] Audit the resulting kernel.config for defconfig surprises (=m promotions, phantom drivers — the Pi-trap lesson generalizes: grep for gadget zoo/hwsim/console assumptions)
- [ ] Full gosd build-kernel run (backgrounded, colima up first), record kernel.config + Image/DTB sizes in kernel/README.md
- [ ] Quality gates + PR

## Progress

Spec/fragment/tests landed (this PR): internal/kernelspec cubie-a5e KernelSpec (fleet tag v6.18.37, RequiredY hand-maintained from gosd-jpc8's compatible-to-driver map, no DTSPatches), build/boards/cubie-a5e/kernel/{kernel-fragment.config,kernelassets.go,README.md}, and the three board-enumerating lists in kernelspec_test.go (board count, DTS-patch allowlist, outputs-vs-Artifacts map). Fragment symbols (pinctrl sun55i-a523 plus _R, MMC_SUNXI, STMMAC_ETH+DWMAC_SUN8I+REALTEK_PHY, MUSB gadget path adapted from rock-4se/radxa-zero-3e's dwc3, EHCI/OHCI platform for the two host ports, AXP717/AXP323 PMIC stack, RTC_DRV_SUN6I, SERIAL_8250_DW) were verified against the pinned tree at v6.18.37 via kernel.googlesource.com, not assumed. Header I2C/SPI DTS patches stay undone per the epic's locked deferral (no SPI node at all in the dtsi at this tag). Quality gates (go test/vet, gofmt, golangci-lint x2) all green.

Pending, left to the orchestrator per this bean's own todos: the real gosd build-kernel --board cubie-a5e run, committing its kernel.config, and the defconfig-surprise audit (=m promotions, phantom drivers, gadget-zoo/hwsim-style traps) - to be pushed to this branch before review.

Base-branch note: PR #184 (bean/gosd-o7jv, this branch's stack base) merged to main mid-task and its branch was deleted; this branch's tip is already an ancestor of main, so no rebase was needed - PR opened against main directly instead of the deleted branch.
