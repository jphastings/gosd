---
# gosd-7bwv
title: Bench-verify an ext4 GOSD-DATA image on real hardware
status: todo
type: task
priority: normal
created_at: 2026-08-09T09:35:37Z
updated_at: 2026-08-10T07:13:02Z
---

gosd-95yu shipped opt-in ext4 for GOSD-DATA verified under qemu only. Boot a real board (rock-4se or radxa-zero-3e — both have CONFIG_EXT4_FS) from an ext4 --data-size=expand image and from a fixed --data-size image: confirm the first-boot grow fills the partition, that a hard power cut mid-write leaves a mountable filesystem with journal replay on the next boot, and that re-flashing over the top re-adopts the partition with its data intact. Mirrors gosd-vv5o, which is the same outstanding bench check for disk/'s ext4 default.



## Bench-verified on pi-zero-2w (2026-08-09/10)

Done on a Raspberry Pi Zero 2W rather than this bean's originally-scoped rock-4se/radxa-zero-3e, because the live question was whether ext4 /data works on the Pi family at all (bean gosd-ssth). Image: examples/hello, --data-filesystem=ext4 --data-size=expand, --hostname pi-ext4, stock artifacts v0.10.0 kernel, 15.9GB card.

The board's serial console was dead throughout (separate wiring fault; latent kernel suspect recorded in gosd-ehkt), and macOS cannot mount ext4, so NOTHING here was read off the filesystem directly. Two channels carried the whole verification: the MBR partition table read with diskutil, and the app's own HTTP endpoint over WiFi.

1. FIRST-BOOT FORMAT/GROW/MOUNT — proven. The flashed image contains only partition 1 (272MiB boot). After boot the card reads p1 hello-boot FAT32 + p2 Linux 15.6GB. In dataexpand's expand path WriteMBR runs LAST, only after FormatEXT4 -> SyncDevice -> EstablishEXT4 -> SyncDevice all return clean, and establishEXT4 itself mounts the fs, grows it via EXT4_IOC_RESIZE_FS, writes+fsyncs the marker into it, then unmounts. So a full-size p2 existing is proof the Pi loaded the ext4 driver, mounted ext4 rw, grew the 512MiB golden to fill the card, and wrote a file into it.

2. RE-ADOPTION, NOT REFORMAT — proven. First HTTP read after WiFi came up returned boots=10, following roughly nine boots that included several reflashes (each of which wipes p2's MBR entry while leaving the data region untouched). A reformat on any of them would have restarted the counter at 1. A later controlled reflash again brought p2 back at 15.6GB.

3. CRASH DURABILITY + JOURNAL REPLAY — proven, and tighter than CI. 'sdwire power off' is a true abrupt mains cut, no clean shutdown. Cut #1: boots=10 -> next boot boots=11. Cut #2 deliberately sharpened — power cut 5.7s after the app reported boots=12 (uptime 5.710685935s), well inside Linux's 30s dirty_expire_centisecs window, so periodic writeback cannot explain survival; next boot returned boots=13. That only reads back if hello's write-fsync-rename (docs/runtime.md) reached the card through ext4's journal, which then replayed cleanly on mount.

NOT covered here: the fixed --data-size shape (only --data-size=expand was run on hardware; CI's qemu-data-ext4 covers fixed under qemu), and rock-4se/radxa-zero-3e themselves, which this bean originally named — an arm64 board still deserves the same pass.

BENCH GOTCHA worth knowing before repeating this: hand-edited WiFi credentials live in gosd.toml on the FAT boot partition, so EVERY reflash wipes them and the board goes silent on the network with nothing wrong. Bake them into the image with 'gosd build --wifi-ssid/--wifi-pass' for any test that reflashes more than once.
