---
# gosd-899s
title: 'Decide: is onboard eMMC support viable without on-device formatting?'
status: completed
type: task
priority: normal
created_at: 2026-07-10T06:15:38Z
updated_at: 2026-07-10T07:53:42Z
parent: gosd-jge2
---

Should GoSD support the onboard eMMC on the Rockchip boards (NanoPi Zero2,
Radxa Zero 3E) at all — given we cannot currently format it on-device, and it
cannot practically be formatted anywhere else?

This bean gates the eMMC mount-library work ([[gosd-tdcc]]); that work is parked (draft) until
this is decided.

## The problem

Investigation (2026-07-10) into an app-facing "mount the eMMC" helper found:

- **The block device already appears.** Both Rockchip kernels compile in the
  eMMC drivers (`CONFIG_MMC_BLOCK`, `CONFIG_MMC_SDHCI_OF_DWCMSHC`); eMMC shows
  up as `/dev/mmcblk0` (sdhci/dwcmshc), microSD as `/dev/mmcblk1`
  (sdmmc/dw_mmc). No kernel or artifact-release work is needed to *see* it.
- **But the kernel is VFAT-only.** No ext4/f2fs/exfat in either
  `build/boards/*/kernel/kernel-fragment.config`. Adding one is a
  kernel-fragment change → the artifact-release "tag-first" dance.
- **There is no on-device format path, of any filesystem.** No pure-Go
  `mkfs` exists in the tree; `go-diskfs` only formats FAT32 at *image-build*
  time on the host, and the kernel cannot format. This is the same wall bean
  [[gosd-xelb]] documented for `/data` ("no pure-Go mkfs.ext4… FAT is the
  honest v1").
- **The killer, raised by JP:** the eMMC is soldered to the board. Unlike a
  microSD you can't pull it and format it on a laptop. So the original
  "mount a pre-formatted FAT eMMC" plan is nearly useless — in practice
  *nothing* ever formats a factory-blank eMMC, so there's no filesystem to
  mount. A mount-only library would return "unformatted, go format it
  elsewhere" on every real board.

**Conclusion driving this decision:** for eMMC to be useful we almost
certainly need a pure-Go on-device `mkfs` (FAT, since the kernel is
VFAT-only) or an equivalent way to lay down a filesystem, before an app can
mount and use the eMMC.

## Options

- **A — Pure-Go on-device mkfs.vfat.** Partition (optional) + FAT32-format
  `/dev/mmcblk0` on the device, then mount. Keeps `CGO_ENABLED=0`.
  Tractability hinges on whether `go-diskfs` can target a live block-device
  node (`/dev/mmcblkN`) rather than an image file — it works over a
  `ReadWriteSeeker`, so plausibly yes; needs a spike to confirm it can create
  a partition table + FAT32 against the real device. Pairs with the mount
  library to make eMMC genuinely usable. **Recommended if eMMC is wanted.**
- **B — USB mass-storage gadget.** Expose the raw eMMC over USB so a tethered
  host formats it. Only works while plugged into a computer, awkward one-time
  UX, and still needs gadget plumbing. A fallback, not a primary path.
- **C — ext4 in-kernel + pure-Go mkfs.ext4.** No pure-Go mkfs.ext4 exists;
  far harder than FAT, and drags in the artifact-release dance for the kernel
  change. Rejected direction.
- **D — Don't support onboard eMMC (yet).** Document it as unsupported;
  revisit if/when a pure-Go mkfs lands. Cheapest; costs users the onboard
  storage on the two Rockchip boards.

## Recommendation

If onboard eMMC is a feature we want for v0.3+, go with **A**: spike a pure-Go
FAT32 mkfs-on-block-device (via `go-diskfs` against `/dev/mmcblkN`), which then
unblocks a combined format+mount library. If it isn't worth the effort now,
pick **D** and park the mount-library bean indefinitely.

## Decision needed
JP to choose A / B / C / D. Record the choice and rationale here, then either
(A) create the pure-Go mkfs spike/impl bean and re-scope the mount library to
format-or-mount, or (D) scrap the mount-library bean.

## Decision (2026-07-10, JP)

Chosen: **option A** — invest in a pure-Go on-device `mkfs.vfat` so a blank
soldered eMMC can be formatted on the device itself, then mounted. Viability of
A hinges on go-diskfs being able to format a real block device, which is now
spiked in [[gosd-0s0m]]. Once that lands, re-scope [[gosd-tdcc]] from mount-only
to format-or-mount.
