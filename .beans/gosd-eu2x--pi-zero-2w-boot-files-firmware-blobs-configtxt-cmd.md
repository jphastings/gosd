---
# gosd-eu2x
title: 'Pi Zero 2W boot files: firmware blobs, config.txt, cmdline.txt, WiFi firmware manifest'
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T20:56:21Z
updated_at: 2026-07-03T17:52:31Z
parent: gosd-vmgw
---

Assemble everything except the kernel that the Pi FAT partition and initramfs need, with pinned sources.

1. GPU boot firmware, pinned to a raspberrypi/firmware release tag in a manifest file `build/boards/pi-zero-2w/manifest.json` with URL + sha256 per file: `bootcode.bin`, `start.elf`, `fixup.dat`.
2. WiFi firmware for the Zero 2W radio (Synaptics/CYW43436): `brcmfmac43436-sdio.bin`, `brcmfmac43436-sdio.txt`, `brcmfmac43436-sdio.clm_blob` (plus the 43430b0 variant files if the RPi firmware-nonfree repo ships them for zero2w — check the repo, some board revisions use 43430b0). Source: github.com/RPi-Distro/firmware-nonfree, pinned commit, sha256 in the same manifest. These go into the initramfs under `/lib/firmware/brcm/`, NOT the FAT partition — wire via the board profile FirmwareFiles.
3. `config.txt` template (locked content): `arm_64bit=1`, `kernel=kernel8.img`, `initramfs initramfs.cpio.zst followkernel`, `enable_uart=1`, `disable_splash=1`, `boot_delay=0`, `avoid_warnings=1`.
4. `cmdline.txt` template (single line, locked): `console=serial0,115200 quiet init=/init gosd.board=pi-zero-2w`

- [x] manifest.json + fetch-and-verify helper (Go, reused later by artifact pipeline; verify sha256 on download)
- [ ] Fill in the pi-zero-2w board profile BootFiles/FirmwareFiles using the manifest (deferred: depends on the `Board` interface from gosd-3zrc, which doesn't exist yet — this bean stays in-progress until gosd-3zrc lands and this can be wired up)
- [x] Templates as go:embed, unit test rendering

## Acceptance
Board profile produces a complete FAT file map (firmware + kernel + config.txt + cmdline.txt + initramfs) and firmware map; all downloads sha256-verified.

## Decision note (2026-07-02)
Blob policy confirmed: firmware blobs are downloaded by the CLI from upstream (raspberrypi/firmware, RPi-Distro/firmware-nonfree) at pinned URL+sha256 and cached — they are NOT bundled into our artifact releases. The manifest.json + fetch-and-verify helper in this task is that mechanism.

## WiFi firmware finding: 43436 vs 43430b0 (2026-07-03)

Checked github.com/RPi-Distro/firmware-nonfree at pinned commit `9794282eb9f4a2de1f23b41a738926740e975d83`. It's a Debian-packaging repo (only a `debian/` tree); the actual blobs live under `debian/config/brcm80211/brcm/` and are tracked directly in git (listed in `debian/source/include-binaries` so dpkg-source stores them verbatim).

For the Zero 2 W (`raspberrypi,model-zero-2-w` compatible string), the brcmfmac driver's board+chip-id firmware lookup resolves to real blobs via git symlinks:

- chip id **43436** → `brcmfmac43436-sdio.{bin,clm_blob,txt}` (real blob, 416101-byte .bin)
- chip id **43430b0** → symlinked to the *same* `brcmfmac43436-sdio.{bin,clm_blob,txt}` blob (not a distinct binary)
- chip id **43430** (no b0) → symlinked to a *different* real blob, `brcmfmac43436s-sdio.{bin,txt}` (442211-byte .bin, no separate `.clm_blob` — its calibration data is baked into the .bin)

So "43436 vs 43430b0" isn't actually a fork in the data: 43430b0 units get the 43436 blob. The real fork is 43436/43430b0 vs plain 43430, which uses a second, separate blob set. Since the Zero 2 W ships with any of these three chip-id revisions in the field and the driver picks the firmware name at runtime from the probed chip ID, both real blob sets (`brcmfmac43436-sdio.*` and `brcmfmac43436s-sdio.*`) are pinned in the manifest, verified, and downloaded — 5 files total. The manifest also records the 8 symlink alias names (`<chip-id>-sdio.raspberrypi,model-zero-2-w.<ext>` → base blob) a board profile needs to materialize under `/lib/firmware/brcm/` (as symlinks or copies) so whichever chip a unit has, its firmware request resolves. That wiring is left to whoever implements `FirmwareFiles` for gosd-3zrc; this bean only pins and verifies the source bytes.

## Summary of Changes

- Added `build/boards/pi-zero-2w/manifest.json`: pins `raspberrypi/firmware` tag `1.20260521` (commit `09267f5354d40519d82fbd2193b9e211ec304055`) for `bootcode.bin`/`start.elf`/`fixup.dat`, and `RPi-Distro/firmware-nonfree` commit `9794282eb9f4a2de1f23b41a738926740e975d83` for the two real WiFi blob sets (`brcmfmac43436-sdio.*`, `brcmfmac43436s-sdio.*`) plus the 8 alias filenames a board profile must materialize. Every URL was fetched live and its sha256 verified independently against the recorded digest — no placeholders.
- Added `internal/fetch`: stdlib-only `fetch.ToDir(ctx, client, File, cacheDir, name)` — GETs a pinned URL, verifies sha256, atomically renames into the cache dir, skips the network call on a cache hit, replaces a stale/corrupt cache entry. Covered by httptest-based tests including the corrupted-checksum case.
- Added `internal/boards/pizero2w/templates`: go:embed `text/template` sources for `config.txt` and `cmdline.txt` with the bean's locked content, rendered via `RenderConfigTxt`/`RenderCmdlineTxt`; unit tests assert the exact locked output and that cmdline.txt renders as a single line.
- Left "Fill in the pi-zero-2w board profile BootFiles/FirmwareFiles" unchecked: it needs the `Board` interface from gosd-3zrc, which doesn't exist yet. Bean stays in-progress until that lands.
