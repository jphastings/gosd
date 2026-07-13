---
# gosd-cuym
title: 'Board support: Radxa ROCK 4SE'
status: todo
type: epic
created_at: 2026-07-13T12:07:32Z
updated_at: 2026-07-13T12:07:32Z
---

Add the Radxa ROCK 4SE (board ID `rock-4se`, RK3399-T SoC, arm64) as a public GoSD board. Driving use case: the betamin appliance (separate, unreferenced repo) — NFC-triggered fullscreen video playback from an NVMe SSD. Planned 2026-07-13; decomposition mirrors the NanoPi Zero2 epic (gosd-cwjf).

## Locked decisions

- Boot chain identical to radxa-zero-3e (BootROM → idbloader @ sector 64 → u-boot.itb @ 8MiB → extlinux); `internal/image` needs zero changes.
- **First blob-free Rockchip board**: no rkbin blobs. U-Boot TPL does open-source DRAM init; BL31 is compiled from mainline Trusted-Firmware-A (`make PLAT=rk3399`). manifest.json records TF-A source (repo/tag/license BSD-3-Clause, compiled-not-pinned) instead of rkbin blobs.
- Upstream support confirmed: Linux `arch/arm64/boot/dts/rockchip/rk3399-rock-4se.dts` (upstream since ~6.3, present at fleet tag v6.18.37); U-Boot `rock-4se-rk3399_defconfig` (since 2023).
- SD boot only. Onboard WiFi/BT **out of scope** for this epic — follow-up bean when needed.
- Stock kernel includes: NVMe/PCIe, exFAT (+NLS deps), USB gadget incl. `CONFIG_USB_CONFIGFS_MASS_STORAGE`. Rationale: M.2 NVMe is a headline board feature; recipe-only NVMe would force every SSD-touching app through Docker. DRM/rkvdec/ALSA stay **out** of stock (fleet trim policy) — video is developer custom-kernel-recipe territory.
- Header I2C/SPI enabled via kernel-build DTS patches (per-SoC convention), not runtime overlays.
- Reserve `rock-4se` in CLAUDE.md's Board IDs locked-decision list in this epic's first PR.
- Boot time: best effort in this epic; A-bring-up records a power-on→/app baseline for a later dedicated optimization bean.
