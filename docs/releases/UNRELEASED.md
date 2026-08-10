# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.2.0 (2026-08-09).

## Breaking changes

(none yet)

## Other call-outs

- **`gosd build --data-filesystem=ext4` now works on the Raspberry Pi boards**
  (bean `gosd-ssth`). v0.2.0's notes said it was "refused at build time for
  boards whose kernel lacks `CONFIG_EXT4_FS` (the Raspberry Pi family)" —
  that was wrong about the shipped kernels. The Pi kernels have built
  `CONFIG_EXT4_FS=y` since artifacts v0.10.0 (bean `gosd-19kw`), which this
  CLI already pins; the refusal came from reading stale committed
  `kernel.config` snapshots. Nothing about the kernels changed, so an image
  built with this release simply stops being refused. FAT32 remains the
  default on every board.
  - **Verified to different depths per board.** Pi Zero 2W has been
    bench-booted end to end — first-boot format, grow and mount, re-adoption
    of its data across a reflash, and a hard power cut taken five seconds
    after an fsync'd write, with the counter surviving (bean `gosd-7bwv`).
    Pi 3B shares the Zero 2W's arm64 kernel pin and rides that same
    released-kernel evidence without a bench pass of its own. **Pi Zero W is
    the fleet's only 32-bit board and has never run ext4 on real hardware**
    — treat it as the least proven of the three.
  - Choosing ext4 still costs you host readability: an ext4 `/data` cannot be
    read or repaired from a macOS or Windows machine, which is why FAT32 is
    still the default, and the filesystem choice remains part of the app's
    on-card ABI (changing it between releases re-establishes `/data`).
