---
# gosd-9sc4
title: 'emmc/: ext4 via the same Filesystem token and default as disk/'
status: todo
type: feature
priority: normal
created_at: 2026-08-07T19:10:53Z
updated_at: 2026-08-07T19:10:53Z
parent: gosd-lfu0
---

JP (2026-08-07): eMMC should get ext4 like disk/ did — internal storage is exactly where the crash-safety argument applies most. This REPLACES the "emmc is FAT32-only by design" locked decision; CLAUDE.md's Public API section must be updated in the same PR.

## Locked decisions

- Mirror disk/'s surface exactly: emmc grows the same typed Filesystem token (ext4/fat32/exfat) with **zero value = ext4** — the same deliberate breaking default, shipped in the same CLI minor release as disk/'s flip (bean gosd-2194's release notes must cover both).
- Implementation should be mostly wiring: internal/blockmount's runEXT4 (golden copy → sync → mount → EXT4_IOC_RESIZE_FS grow → marker, PR #192) is already fs-parameterized and shared — emmc stops pinning diskfmt.FAT32 and passes the token through. Any place the emmc path structurally diverges from disk's ext4 path (candidate selection, eMMC device naming, boot-partition exclusion rules) gets called out explicitly, not silently absorbed.
- Kernel support is already there on every eMMC-bearing board (Rockchip fleet has EXT4_FS=y; the /proc/filesystems preflight still guards).
- Tests mirror gosd-1c0x's fake-driven state-machine set, emmc-flavored; the emmc-specific "provably unchanged FAT32 semantics" tests from #192 are superseded and updated to the new contract.
- fs-match adoption rule (from #192) now applies to emmc with ext4 requestable: an established FAT32 emmc volume + default(ext4) request → refuse without destructive; document the upgrade story in the release note (reformat is data loss — apps opt in).
