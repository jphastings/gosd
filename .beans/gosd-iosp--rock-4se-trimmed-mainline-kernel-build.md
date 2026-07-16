---
# gosd-iosp
title: 'ROCK 4SE: trimmed mainline kernel build'
status: completed
type: task
priority: normal
created_at: 2026-07-13T12:25:00Z
updated_at: 2026-07-16T16:05:15Z
parent: gosd-cuym
blocked_by:
    - gosd-je2r
---

Mirror `build/boards/radxa-zero-3e/kernel/` + its kernelspec entry for rock-4se: `kernelassets.go` (//go:embed), `kernel-fragment.config`, `patches/`, committed generated `kernel.config` (GPL provenance header), README; new entry in `internal/kernelspec/kernelspec.go` (fleet repo/tag v6.18.37, `defconfig`, arm64 toolchain, DTB target `rockchip/rk3399-rock-4se.dtb`, `KernelFilename: "Image"`, ModulesDisabled). Adds the rock-4se kernel job to `.github/workflows/build-artifacts.yml` (no artifacts.Version bump here — tag-first, bump-second).

## Locked decisions

- Fragment beyond the Rockchip trim baseline: `CONFIG_PCI`/`CONFIG_PCIE_ROCKCHIP_HOST`/`CONFIG_PHY_ROCKCHIP_PCIE`, `CONFIG_BLK_DEV_NVME`, `CONFIG_EXFAT_FS` (+NLS deps) — the betamin SSD is exFAT (host-native for USB mass-storage exposure).
- USB gadget: mirror radxa-zero-3e's gadget enablement (dwc3 dual-role, configfs) **plus `CONFIG_USB_CONFIGFS_MASS_STORAGE`**.
- RK3399-specifics from A1's findings: SD controller driver, USB PHYs (INNO_USB2, Type-C), GbE (DWMAC_ROCKCHIP + PHY).
- DRM/SOUND/WLAN/MEDIA stay `is not set` (fleet trim policy; video is developer-recipe territory).
- DTS patches: `status="okay"` for header I2C bus(es) + header SPI with spidev child node (accepted compatible), per the per-SoC peripheral convention; dwc3 dr_mode patch if A1 found host-only.
- RequiredY mirrors the fragment's additions (hand-maintained literals, nanopi style).
- `TestKernelSpecOutputsMatchBoardArtifacts` binds artifact names to the A4 board profile — coordinate names with A4.

## Todo

- [x] kernelspec entry + build/boards/rock-4se/kernel/ scaffolding
- [x] Fragment + RequiredY (baseline trim + NVMe/exFAT/gadget/mass-storage + A1 specifics)
- [x] DTS patches (header I2C, header SPI+spidev, dr_mode if needed); confirm each applies at the pinned tag
- [x] Real `gosd build-kernel --board rock-4se` Docker build; commit generated kernel.config; spot-check symbols =y
- [x] CI kernel job in build-artifacts.yml (unblocked by folding gosd-0vvh's board registration onto this branch — see coupling note below)

## Summary of Changes

Branch `bean/gosd-iosp-rock4se-kernel` (also carries gosd-0vvh's board
registration; coupling note below).

- `build/boards/rock-4se/kernel/`: `kernelassets.go` embeds, `kernel-fragment.config` (radxa-zero-3e mirror + RK3399 specifics: dw_mmc-family SD, INNO_USB2 + TYPEC USB PHYs, stmmac GbE + Realtek PHY, PCIe + NVMe, exFAT, configfs mass-storage gadget), three DTS patches (header I2C i2c2/i2c6/i2c7; SPI1 + spidev child; dwc3_0 → `dr_mode = "peripheral"`), README. Every `CONFIG_*` symbol verified against the pinned tree's Kconfig at v6.18.37; all three patches generated from and test-applied (GNU patch, `--fuzz=0`) against the real `rk3399-rock-4se.dts` at that tag.
- `internal/kernelspec/kernelspec.go`: `"rock-4se"` entry (fleet tag v6.18.37, `defconfig`, arm64 toolchain, DTB `rockchip/rk3399-rock-4se.dtb`, `KernelFilename: "Image"`, `ModulesDisabled: true`, full RequiredY incl. `CONFIG_PHY_ROCKCHIP_TYPEC` — not the RK3566-only combo-phy symbol, per gosd-je2r).
- **Real build succeeded** (2026-07-16, run on the remote build box per CLAUDE.md's colocated-docker rule): committed the generated `kernel.config`. Spot-checks passed: every RequiredY symbol `=y` in the generated config; built DTB carries `dr_mode = "peripheral"` (patch 0003 confirmed effective). Sizes: `Image` 68 MB, `rk3399-rock-4se.dtb` 63 KB.
- CI: `rock-4se-kernel` job + package-and-release wiring (needs + staging download) in build-artifacts.yml. The release `files:` entry for `dist/rock-4se.tar.zst` lands with gosd-dtpo's uboot job, once the tarball's contents are complete.

**Cross-bean coupling (discovered here, now recorded in CLAUDE.md):**
`gosd build-kernel --board <id>` resolves the board through `internal/boards`
*before* its kernelspec lookup, so the kernel was unbuildable until
gosd-0vvh's `internal/boards/rock4se/` profile + `RegisterInternal` existed.
That core slice of gosd-0vvh was folded onto this branch; gosd-0vvh retains
its remaining scope (board_test.go, build_integration_test.go extension,
docs/board-build-tags.md entry).

**Flagged uncertainties (for A6 bring-up; also noted in patch/README comments):**
- Patch 0003 flips `usbdrd_dwc3_0` to peripheral as a **best guess** for which
  dwc3 maps to the physical host/device-switch OTG port (gosd-je2r couldn't
  resolve it from DTS text — the shared dtsi treats both symmetrically). Swap
  to `usbdrd_dwc3_1` if bring-up disagrees.
- 40-pin header pin mapping (I2C 3/5, 27/28, 29/31; SPI 19/21/23/24) is from
  Radxa's docs, not an opened schematic — re-check if a peripheral doesn't
  enumerate.
