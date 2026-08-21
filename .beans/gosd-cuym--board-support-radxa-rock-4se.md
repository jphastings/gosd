---
# gosd-cuym
title: 'Board support: Radxa ROCK 4SE'
status: completed
type: epic
priority: normal
created_at: 2026-07-13T12:07:32Z
updated_at: 2026-08-21T01:35:57Z
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

## Summary of Changes

Shipped `rock-4se` as a public GoSD board, and GoSD's first **blob-free
Rockchip** target: no rkbin at all — U-Boot's TPL does open-source DRAM init
and BL31 is compiled from mainline Trusted-Firmware-A (`PLAT=rk3399`), with
the manifest recording TF-A's repo/tag/licence instead of pinned binaries
(gosd-dtpo). gosd-je2r confirmed upstream kernel and U-Boot support for the
RK3399-T before any build work started; gosd-iosp built the trimmed mainline
kernel, deliberately including NVMe/PCIe and exFAT because M.2 storage is the
board's headline feature; gosd-0vvh added the board profile (extlinux plus
`idbloader.img`/`u-boot.itb` raw writes, needing no `internal/image` change);
gosd-h8a8 cut the artifacts release and flipped the board public in one
activation PR; gosd-sz6p brought it up on hardware.

Onboard WiFi/BT stayed out of scope as the epic locked. Header I2C/SPI are
enabled by kernel-build DTS patches, per the per-SoC convention.

The board is registered publicly in `internal/boardset`, has a `kernelspec`
entry, a `build/boards/rock-4se/` tree, fake-artifact fixtures and a
"Complete" bring-up row in COMPATIBILITY.md — parity enforced by
`internal/repocheck`.

Its NVMe ext4 power-cut bench verification is deliberately NOT part of this
epic: it belongs to the ext4 epic as gosd-vv5o, which remains open.
