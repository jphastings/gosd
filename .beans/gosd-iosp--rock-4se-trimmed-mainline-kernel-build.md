---
# gosd-iosp
title: 'ROCK 4SE: trimmed mainline kernel build'
status: todo
type: task
priority: normal
created_at: 2026-07-13T12:25:00Z
updated_at: 2026-07-13T13:26:09Z
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

- [ ] kernelspec entry + build/boards/rock-4se/kernel/ scaffolding
- [ ] Fragment + RequiredY (baseline trim + NVMe/exFAT/gadget/mass-storage + A1 specifics)
- [ ] DTS patches (header I2C, header SPI+spidev, dr_mode if needed); confirm each applies at the pinned tag
- [ ] Real `gosd build-kernel --board rock-4se` Docker build; commit generated kernel.config; spot-check symbols =y
- [ ] CI kernel job in build-artifacts.yml
