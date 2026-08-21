---
# gosd-vi0n
title: CLI & pure-Go image assembly
status: completed
type: epic
priority: normal
created_at: 2026-07-02T20:49:51Z
updated_at: 2026-08-21T01:36:45Z
parent: gosd-sc9w
---

The `gosd` CLI: cross-compile the user's Go main package and assemble a bootable SD image entirely in pure Go — no root, no loop mounts, no Docker. This is the core promise (works identically on macOS/Linux/CI).

Locked decisions (do not relitigate in child tasks):
- Root filesystem is an **initramfs (cpio, zstd-compressed)** containing the app binary, /lib/firmware blobs, and nothing else. No squashfs, no ext4 root, everything runs from RAM. Userland is one static Go binary + gosd-init.
- Image layout shared by both boards: MBR partition table; partition 1 = FAT32, label GOSD-BOOT, starting at 16MiB (leaves LBA 64–16MiB gap free for the Rockchip bootloader); no other partitions in v0.1.
- Filesystem/partition writing via github.com/diskfs/go-diskfs; cpio via github.com/u-root/u-root/pkg/cpio.
- User app is compiled with CGO_ENABLED=0 GOOS=linux GOARCH=arm64 (both boards are arm64).

## Summary of Changes

Delivered the core promise: `gosd build` cross-compiles a user's Go main
package and assembles a bootable SD image entirely in pure Go — no root, no
loop mounts, no Docker — identically on macOS, Linux and CI. gosd-56xt
scaffolded the CLI and the `CGO_ENABLED=0` cross-compile; gosd-vq4g built the
initramfs writer (newc cpio via u-root, zstd via klauspost); gosd-cvzt the MBR
plus FAT32 image writer over go-diskfs; gosd-3zrc the board-profile
abstraction that ties them together end to end; gosd-zplh settled how gosd
locates gosd-init's source when run outside its own repo (local checkout, else
`go mod download` at gosd's own version, with `--gosd-init-src` as the escape
hatch); gosd-m0vj shipped `examples/hello`; gosd-1937 added the per-board
build tags that let an app gate source per board.

The layout has since grown past this epic's v0.1 sketch — a data partition, a
config tree, per-app labels, per-board architectures — but the assembly model
laid down here (everything runs from RAM off one initramfs; one static Go
binary plus gosd-init; pure-Go partition and filesystem writing) is unchanged
and is what every later board and feature builds on.
