---
# gosd-1c0x
title: 'disk/blockmount: Filesystem token (ext4 default), mount + online grow, adoption gate'
status: todo
type: task
priority: normal
created_at: 2026-08-07T09:58:20Z
updated_at: 2026-08-07T09:58:28Z
parent: gosd-lfu0
blocked_by:
    - gosd-apmv
---

The API + runtime half of epic gosd-lfu0. disk's Options gains the typed filesystem token (ext4/fat32/exfat; zero value = ext4 — the locked breaking default), FormatAndMount/FormatAndMountWith flow it through internal/blockmount, and the linux side learns: mount -t ext4, then EXT4_IOC_RESIZE_FS online grow to the partition size, then the establishment marker. Kernel support preflight via /proc/filesystems (same as exFAT) with an actionable per-board error naming the Pi gap. emmc/ is NOT touched (FAT32-only stands) — blockmount changes must not shift emmc semantics (shared-package rule in CLAUDE.md).

## Todos

- [ ] Options.Filesystem typed token + docstrings recording the ext4 default and the deliberate behavior break (minor bump at ship)
- [ ] blockmount: candidate selection/label rules for ext4 (16-byte label cap), fs-match rule on adoption (mismatch: reformat iff Destructive, else actionable error)
- [ ] platform_linux.go: mount, RESIZE_FS ioctl grow (partition size derived from the block device, never assumed), sync/marker ordering; platform_other.go stubs; fake-driven tests for the full state machine incl. crash-debris cases (golden partially written; grown but marker missing; marker present)
- [ ] Explicit crash-ordering argument in the package docs (write → sync → marker → sync) + adversarial self-review BEFORE requesting JP review — probe-only adoption is the named historical failure (gosd-lirl)
- [ ] Growth-after-adoption: re-mounting an established volume must NOT re-grow or re-format; only first establishment grows
- [ ] Quality gates + PR
