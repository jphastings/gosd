---
# gosd-f1je
title: 'example: eMMC website over HTTP, shared over USB mass storage'
status: completed
type: feature
priority: normal
created_at: 2026-07-13T17:42:56Z
updated_at: 2026-07-13T20:08:41Z
parent: gosd-jge2
blocked_by:
    - gosd-k2fs
---

An example app demonstrating `gadget.MassStorage` (bean gosd-k2fs) end to end: on a board with onboard eMMC AND USB gadget support (radxa-zero-3e today; rock-4se later), it serves the eMMC's contents as a static website over HTTP, but when plugged into a computer it exposes the eMMC as a USB drive so the site's files can be edited, then reverts to serving on the next standalone boot.

Stacked on #82 (gosd-k2fs); branch from that PR's branch, rebase onto main once it lands.

## Locked decisions

- **emmc API change (requested by JP):** `emmc.FormatAndMount` returns `<-chan Result` where `Result{MountPoint, BlockDevice string; Err error}`, so callers get the block device backing the mount to hand to `gadget.MassStorage`. Breaking change to the public emmc package (v0.x, minor bump); the sole in-repo caller (examples/emmcstorage) is updated in the same PR.
- **Partition question (answered):** f_mass_storage backs a LUN with any block node (partition, whole device, or image file). The emmc package formats whole-device FAT (no partition table), so `BlockDevice` is the whole device `/dev/mmcblk0` — exposed over MSC it presents as a normal removable FAT drive. Not a blocker; no repartitioning of the emmc format model.
- **New `emmc.Unmount(mountpoint)`:** small companion needed because exposing the device over MSC requires it unmounted first (expose XOR mount, per the massstorage docs). platform_linux + platform_other split like the rest of the package.
- **Mode = decide once at boot** (not reactive hot-swap): format+mount; if the USB controller reaches "configured" (a computer enumerated us) expose the eMMC as a drive and stay there; otherwise serve the mount over HTTP. Honest given no board has had hardware bring-up.
- Idempotent format (never wipes existing content); graceful degradation (no eMMC → log + exit like emmcstorage; no UDC → just serve).
- Requires `gosd build --usb-gadget`. Target board: radxa-zero-3e.

## Todo

- [x] emmc: FormatAndMount returns Result{MountPoint,BlockDevice,Err}; run returns (device, err); update tests
- [x] emmc: add Unmount (platform_linux + platform_other)
- [x] Update examples/emmcstorage for the new Result API
- [x] New examples/usbwebsite: format+mount, host-detect via /sys/class/udc state, MSC-or-serve
- [x] README for the example (build --usb-gadget, board support, power/mode behavior)
- [x] docs/runtime.md + COMPATIBILITY note if warranted
- [x] Quality gates; cross-compile arm64 + armv6; PR stacked on #82

## Summary of Changes

Added `examples/usbwebsite`, the worked example for `gadget.MassStorage`: on a
board with onboard eMMC it serves the eMMC's contents as a static website over
HTTP, but when a computer enumerates it over USB it presents that same eMMC as a
removable drive so the site's files can be edited, reverting to serving on the
next standalone boot. The mode is chosen once per boot (never both at once —
the host writes raw blocks with no knowledge of our filesystem), and a
connected computer is distinguished from a power-only supply by watching the
USB controller reach the "configured" state.

To make the eMMC's block device reachable for MSC, the public `emmc` package
gained:
- `FormatAndMount` now returns `<-chan Result` (`Result{MountPoint, BlockDevice
  string; Err error}`) instead of `<-chan error`, so callers get the device
  backing the mount. `run` returns the device; the warm-restart (already
  mounted) path reports it via `mountedAt` reading /proc/mounts, since discovery
  deliberately skips mounted devices.
- `Unmount(mountpoint)` releases the device so `gadget.MassStorage` can take it
  exclusively (platform_linux + platform_other stub).

Breaking change to the public `emmc` package (v0.x, minor bump); the sole
in-repo caller, `examples/emmcstorage`, is updated in this PR. Docs: a pointer
in docs/runtime.md's USB gadget section and the example's own README (build
`--usb-gadget`, board support, power topology, pre-hardware caveat). CI
cross-compiles the new example for arm64 and armv6.

On the partition question: f_mass_storage backs a LUN with any block node
(partition, whole device, or image file). The eMMC is formatted whole-device
FAT (no partition table), so `BlockDevice` is `/dev/mmcblk0` and presents to a
host as a normal removable FAT drive — no change to the emmc format model.
