---
# gosd-eu2x
title: 'Pi Zero 2W boot files: firmware blobs, config.txt, cmdline.txt, WiFi firmware manifest'
status: todo
type: task
priority: normal
created_at: 2026-07-02T20:56:21Z
updated_at: 2026-07-02T21:17:59Z
parent: gosd-vmgw
---

Assemble everything except the kernel that the Pi FAT partition and initramfs need, with pinned sources.

1. GPU boot firmware, pinned to a raspberrypi/firmware release tag in a manifest file `build/boards/pi-zero-2w/manifest.json` with URL + sha256 per file: `bootcode.bin`, `start.elf`, `fixup.dat`.
2. WiFi firmware for the Zero 2W radio (Synaptics/CYW43436): `brcmfmac43436-sdio.bin`, `brcmfmac43436-sdio.txt`, `brcmfmac43436-sdio.clm_blob` (plus the 43430b0 variant files if the RPi firmware-nonfree repo ships them for zero2w — check the repo, some board revisions use 43430b0). Source: github.com/RPi-Distro/firmware-nonfree, pinned commit, sha256 in the same manifest. These go into the initramfs under `/lib/firmware/brcm/`, NOT the FAT partition — wire via the board profile FirmwareFiles.
3. `config.txt` template (locked content): `arm_64bit=1`, `kernel=kernel8.img`, `initramfs initramfs.cpio.zst followkernel`, `enable_uart=1`, `disable_splash=1`, `boot_delay=0`, `avoid_warnings=1`.
4. `cmdline.txt` template (single line, locked): `console=serial0,115200 quiet init=/init gosd.board=pi-zero-2w`

- [ ] manifest.json + fetch-and-verify helper (Go, reused later by artifact pipeline; verify sha256 on download)
- [ ] Fill in the pi-zero-2w board profile BootFiles/FirmwareFiles using the manifest
- [ ] Templates as go:embed, unit test rendering

## Acceptance
Board profile produces a complete FAT file map (firmware + kernel + config.txt + cmdline.txt + initramfs) and firmware map; all downloads sha256-verified.

## Decision note (2026-07-02)
Blob policy confirmed: firmware blobs are downloaded by the CLI from upstream (raspberrypi/firmware, RPi-Distro/firmware-nonfree) at pinned URL+sha256 and cached — they are NOT bundled into our artifact releases. The manifest.json + fetch-and-verify helper in this task is that mechanism.
