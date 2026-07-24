---
# gosd-pcwl
title: gosd-init can mount the wrong boot partition when eMMC precedes SD
status: todo
type: bug
created_at: 2026-07-24T06:38:28Z
updated_at: 2026-07-24T06:38:28Z
---

Found during NanoPi Zero2 eMMC-refit testing (gosd-odp7, 2026-07-24). With an eMMC fitted, the kernel enumerates it as mmcblk0 and the SD as mmcblk1. gosd-init's boot-partition probe walks candidates by device name (mmcblk0p1 first), so it probed the eMMC's first partition as GOSD-BOOT — kernel FAT driver rejected it ('FAT-fs (mmcblk0p1): bogus number of reserved sectors', retried ~3 times over ~0.8s) and the probe then fell through to the SD's mmcblk1p1, which mounted fine. Failed-safe ONLY because that eMMC partition isn't valid FAT: any eMMC whose p1 IS a valid FAT filesystem (vendor-shipped image, previously-flashed GoSD image) would be mounted as /boot instead of the SD the user just flashed — stale gosd.toml, wrong app config, very confusing failures.

Fix direction: identify GOSD-BOOT by FAT volume label (the partitions are labelled GOSD-BOOT by the image builder) rather than accepting the first mountable FAT by device-name order — check the label before committing, or enumerate by-label via the kernel's vfat label support in gosd-init's probe seam. Also consider logging the DEVICE the boot partition was mounted from ('boot partition mounted at /boot' currently omits the source, which slowed diagnosis).

Repro on bench: NanoPi Zero2 + any eMMC with a valid-FAT first partition + GoSD SD card; without the fix /boot comes from eMMC. Runtime code: gosd-init boot-partition probe (see the mmcblk0p1/mmcblk1p1/vda1 candidate loop in the boot log). Tests: fake-driven per the gosd-init platform seam convention.
