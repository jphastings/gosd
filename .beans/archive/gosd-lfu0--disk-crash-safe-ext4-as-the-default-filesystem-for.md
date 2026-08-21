---
# gosd-lfu0
title: 'disk/: crash-safe ext4 as the default filesystem for internal drives'
status: completed
type: epic
priority: normal
created_at: 2026-08-07T09:57:26Z
updated_at: 2026-08-21T06:50:04Z
---

JP (2026-08-07): FAT on internal drives (eMMC-attached NVMe, USB drives via disk/) is causing real problems — power-cut corruption and the FAT32 size ceiling. Add ext4 as a disk/ filesystem option and make it the DEFAULT: disk.FormatAndMount is almost always used for internal drives, where host-OS readability doesn't matter and crash-safety does.

## Locked decisions (JP, 2026-08-07 conversation)

- **Formatting route: golden-image + kernel online-grow** ("option 1"): a small pristine ext4 image, generated ONCE from e2fsprogs under Docker by a maintainer recipe, checked into the repo compressed with provenance (e2fsprogs version, exact mke2fs invocation) recorded alongside; on-device "format" = decompress + raw-write golden image + set per-volume label/UUID in the superblock (pure Go) + mount + grow to partition size via the EXT4_IOC_RESIZE_FS ioctl. NO mkfs binary on device, NO pure-Go mke2fs, NO resize2fs. gosd-init stays userland-free.
- **API shape**: disk's Options gains a filesystem token (typed string consts: ext4 / fat32 / exfat); **zero value = ext4** — a deliberate BREAKING behavior change to FormatAndMount/FormatAndMountWith's default. Not used externally yet → ship with a MINOR version bump, release-notes-level note. FAT32/exFAT remain available as explicit tokens (removable-media interop).
- **emmc/ is unchanged**: stays FAT32-only by design (its locked decision stands). Only disk/ gains the token. CLAUDE.md's Public API locked decision gets updated wording in the shipping PR.
- **Kernel reality (verified from recorded kernel.configs 2026-08-07)**: EXT4_FS=y already on radxa-zero-3e, nanopi-zero2, rock-4se, qemu-virt (arm64 defconfig inheritance — zero artifacts dance needed for every board that has eMMC/NVMe today). Pi boards: EXT4 not set → disk/ ext4 on a Pi fails with an actionable error via the /proc/filesystems preflight (same gate as exFAT); Pi fragment enablement is a separate follow-up if ever wanted, with the full artifacts release dance.
- **Adoption/crash-ordering**: same rules as every on-disk commit — write → sync → marker → sync; a probe is never proof a format completed (an interrupted golden-image write leaves a probe-passing ext4 superblock early in the image, so the establishment gate must be marker-based). Adversarial review pass required BEFORE JP review. Adoption of an existing established volume: fs type must match the requested token; mismatch → reformat when Destructive, actionable error when not.
- **Durability story unchanged**: the journal buys metadata crash-consistency + mount-time replay, not data durability — docs/runtime.md's fsync pattern remains the app-facing guidance.
- GOSD label rules: ext4 volume labels are 16 bytes max — confirm blockmount's label scheme fits (GOSD-DATA is 9 chars — fine) and document.

## Non-goals

- /data on the SD card stays FAT **by default** (removable interop is the point there; gosd-mt53 tracks its size ceiling separately — ext4 here does NOT subsume it). AMENDED 2026-08-09 by bean gosd-95yu, at JP's request: ext4 GOSD-DATA is now available as an explicit build-time opt-in (`gosd build --data-filesystem=ext4`) for apps that need /data to survive rapid power-off. The non-goal's rationale is preserved — FAT32 remains the default precisely so a flashed card stays readable in any computer's SD reader — and this remains a non-goal as originally written: ext4 is never the /data default.
- F2FS/btrfs: out (no viable no-userland format path; revisit F2FS only if the externals epic gosd-oyhi lands a bundled mkfs.f2fs).
- Pi-family ext4 kernel enablement: out until a concrete need.

Related: rock-4se NVMe is the flagship consumer (betamin appliance). Bench verification on rock-4se NVMe belongs to the final child bean.


## Summary of Changes

Closed 2026-08-21 (JP) under the convention recorded in CLAUDE.md's Workflow
section: an epic whose implementation has shipped and is CI-proven closes even
when a hardware bench verification is still outstanding — the delivered work
gets recorded as delivered, and the outstanding verification keeps its own bean
rather than holding an epic hostage.

Shipped, all on `main`:

- Pure-Go ext4 inspect and format-by-golden-copy in `internal/diskfmt`, plus
  the checked-in 512MiB golden itself with its generator recipe and provenance
  (`internal/diskfmt/ext4golden`) — beans gosd-apmv, gosd-u988.
- `disk/` and `internal/blockmount`: the typed `Filesystem` token with ext4 as
  its zero value, the once-only online grow via `EXT4_IOC_RESIZE_FS`, and the
  marker-gated establishment/adoption state machine that never treats a
  filesystem probe as proof a format completed — bean gosd-1c0x.
- `emmc/` took the same token and the same ext4 default (bean gosd-9sc4), and
  the Pi-family kernels gained `CONFIG_EXT4_FS` so the default works on Pi USB
  drives too (bean gosd-19kw).
- Ship pass: the `qemu-disk-ext4` CI job — format, grow, a hard qemu kill with
  no clean shutdown, reboot, adopt, journal replay — plus docs,
  COMPATIBILITY.md, CLAUDE.md's locked decision and the minor version bump
  (bean gosd-ucgr).

**This closure is not a hardware-verification claim.** Everything above is
proven by unit tests and QEMU only. Real-hardware close-out on the bench —
rock-4se + NVMe SSD + the sdwire power-cut rig — is bean gosd-vv5o, now a
standalone bench bean with no parent.
