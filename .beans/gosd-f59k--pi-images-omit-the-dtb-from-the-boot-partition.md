---
# gosd-f59k
title: Pi images omit the DTB from the boot partition
status: todo
type: bug
created_at: 2026-07-24T19:18:28Z
updated_at: 2026-07-24T19:18:28Z
---

Found during Pi Zero 2W first hardware boot (gosd-m9dj, 2026-07-24). The assembled image's GOSD-BOOT partition contained firmware (bootcode.bin/start.elf/fixup.dat), kernel8.img, initramfs, config.txt, cmdline.txt, gosd.toml — but NO bcm2710-rpi-zero-2-w.dtb, even though the v0.6.0 artifact tarball ships it. Result: firmware boots, kernel8.img loads, and the kernel hangs before any console output (no device tree = no drivers, no UART) — a completely silent failure with healthy-looking ACT-LED activity. Diagnosed by inspecting the flashed card; hand-copying the DTB from the artifact cache onto the partition fixed boot immediately (bench-verified).

Fix: the Pi board assembly path (internal/pipeline + pi board profiles' boot-file lists) must copy the DTB artifact to the FAT partition, presumably for both pi-zero-2w and pi-zero-w (check pi-zero-w's assembly for the same omission — its DTB name differs, bcm2708-rpi-zero-w.dtb or similar per its manifest). Add an integration-test assertion (the build_integration_test.go pattern reads built images back) that each Pi board's boot partition contains its DTB file — the exact assertion that would have caught this. Rockchip boards are unaffected (extlinux fdt line + verified on hardware).

How this survived CI: qemu-virt exercises a different boot path entirely, and no fixture test asserted Pi boot-partition completeness against the artifact manifest.
