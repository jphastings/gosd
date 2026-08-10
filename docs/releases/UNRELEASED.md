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
  - **Verified on hardware, to different depths per board.** Pi Zero 2W was
    bench-booted end to end — first-boot format, grow and mount, re-adoption
    of its data across a reflash, and a hard power cut taken five seconds
    after an fsync'd write, with the counter surviving (bean `gosd-7bwv`).
    Pi Zero W — the fleet's only 32-bit board, and so the first anywhere to
    run GoSD's ext4 path on armv6 — was then bench-booted too: the golden
    grew to 14.6GiB and mounted, and the boot counter survived an abrupt
    power cut and came back adopted rather than reformatted (bean
    `gosd-58p6`). Pi 3B shares the Zero 2W's arm64 kernel pin and rides that
    same released-kernel evidence without a bench pass of its own yet.
  - Choosing ext4 still costs you host readability: an ext4 `/data` cannot be
    read or repaired from a macOS or Windows machine, which is why FAT32 is
    still the default, and the filesystem choice remains part of the app's
    on-card ABI (changing it between releases re-establishes `/data`).

- **Apps can now gate code on being built by GoSD at all, with `//go:build
  gosd`** (bean `gosd-cm4b`). Every app compile gets a plain `gosd` tag
  alongside the existing per-board `gosd_<board-id>`. Previously the only way
  to ask "am I being compiled for a card?" was to negate every board tag in
  turn, which silently rotted the moment a board was added. The common case is
  coarser than per-board anyway — real `/data` and GPIO on a card, versus a
  temp dir and fakes under `go test`.

- **`gosd build` now says so plainly when the Go toolchain is missing or too
  old** (bean `gosd-jm2v`), instead of surfacing whatever the underlying
  `go` invocation happened to print.
