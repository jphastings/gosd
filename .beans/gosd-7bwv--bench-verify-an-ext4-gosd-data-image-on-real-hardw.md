---
# gosd-7bwv
title: Bench-verify an ext4 GOSD-DATA image on real hardware
status: todo
type: task
created_at: 2026-08-09T09:35:37Z
updated_at: 2026-08-09T09:35:37Z
---

gosd-95yu shipped opt-in ext4 for GOSD-DATA verified under qemu only. Boot a real board (rock-4se or radxa-zero-3e — both have CONFIG_EXT4_FS) from an ext4 --data-size=expand image and from a fixed --data-size image: confirm the first-boot grow fills the partition, that a hard power cut mid-write leaves a mountable filesystem with journal replay on the next boot, and that re-flashing over the top re-adopts the partition with its data intact. Mirrors gosd-vv5o, which is the same outstanding bench check for disk/'s ext4 default.
