---
# gosd-ssth
title: Decide whether the Pi family kernels should gain CONFIG_EXT4_FS
status: draft
type: task
created_at: 2026-08-09T09:35:37Z
updated_at: 2026-08-09T09:35:37Z
---

gosd-95yu makes ext4 GOSD-DATA opt-in but refuses it at build time for pi-zero-w, pi-zero-2w and pi-3b, whose stock kernels do not build CONFIG_EXT4_FS. Enabling it would let those boards use a crash-resilient /data too, but it is a family-wide raspberrypi/linux commit-pin change plus the full artifacts release dance, and kernel size on pi-zero-w (armv6, smallest board) is the sensitive constraint — measure the initramfs/kernel growth before committing. JP's call; left as an open question on gosd-95yu rather than assumed.
