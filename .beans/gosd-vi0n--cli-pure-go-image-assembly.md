---
# gosd-vi0n
title: CLI & pure-Go image assembly
status: todo
type: epic
created_at: 2026-07-02T20:49:51Z
updated_at: 2026-07-02T20:49:51Z
parent: gosd-sc9w
---

The `gosd` CLI: cross-compile the user's Go main package and assemble a bootable SD image entirely in pure Go — no root, no loop mounts, no Docker. This is the core promise (works identically on macOS/Linux/CI).

Locked decisions (do not relitigate in child tasks):
- Root filesystem is an **initramfs (cpio, zstd-compressed)** containing the app binary, /lib/firmware blobs, and nothing else. No squashfs, no ext4 root, everything runs from RAM. Userland is one static Go binary + gosd-init.
- Image layout shared by both boards: MBR partition table; partition 1 = FAT32, label GOSD-BOOT, starting at 16MiB (leaves LBA 64–16MiB gap free for the Rockchip bootloader); no other partitions in v0.1.
- Filesystem/partition writing via github.com/diskfs/go-diskfs; cpio via github.com/u-root/u-root/pkg/cpio.
- User app is compiled with CGO_ENABLED=0 GOOS=linux GOARCH=arm64 (both boards are arm64).
