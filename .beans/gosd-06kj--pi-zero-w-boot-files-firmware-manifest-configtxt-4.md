---
# gosd-06kj
title: 'Pi Zero W boot files: firmware manifest, config.txt, 43430 WiFi blobs'
status: todo
type: task
created_at: 2026-07-06T15:48:45Z
updated_at: 2026-07-06T15:48:45Z
parent: gosd-ajpz
---

Mirror the pi-zero-2w boot-files work: manifest.json pinning the SAME raspberrypi/firmware tag (bootcode.bin/start.elf/fixup.dat are shared) + RPi-Distro/firmware-nonfree pinned commit for the brcmfmac43430-sdio blob family (Zero W uses 43430/43438 — check the repo symlinks like the 2W task did and record which aliases /lib/firmware/brcm needs). Real downloaded sha256s, no placeholders. config.txt template: like pi-zero-2w but NO arm_64bit line and kernel=kernel.img; cmdline.txt identical pattern (console=serial0,115200 quiet init=/init gosd.board=pi-zero-w). go:embed templates + render tests in internal/boards/pizerow/templates.
- [ ] manifest.json with verified hashes + alias findings recorded
- [ ] Templates + render tests
