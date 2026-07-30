---
# gosd-8kdm
title: go-diskfs FAT32 formatter silently corrupts volumes past ~256GiB (uint16 sectors-per-FAT)
status: todo
type: bug
created_at: 2026-07-30T20:28:52Z
updated_at: 2026-07-30T20:28:52Z
---

Found while designing --data-size=expand (bean gosd-6sac): go-diskfs v1.9.3's `fat32.Create` computes `sectorsPerFat := uint16((4*(totalSectors-reserved) + fatEntryDenom - 1) / fatEntryDenom)` — a straight uint16 cast. With the 32KiB clusters the >32GB size class uses, the value exceeds 65535 once the volume passes roughly 256GiB, silently truncating: the FAT is laid out far too small for the cluster count and the resulting filesystem is corrupt. `Fat32MaxSize` (2TiB) doesn't catch it.

Reachable today through anything that formats large media via `internal/diskfmt.FormatFAT32`: the public `disk` package with a 512GB/1TB SSD or USB drive attached, and (mitigated) `--data-size=expand`, which caps its created partition at 256GiB with a logged notice (see `cmd/gosd-init/internal/dataexpand`'s `maxPartitionBytes`) until this is fixed.

Fix options: upstream PR to go-diskfs making the sectors-per-FAT computation 32-bit (FATSz32 is a u32 field on disk — uint16 is purely an implementation slip), then bump the pin and delete both caps; or guard in `diskfmt.FormatFAT32` with an actionable error above the safe size so `disk`/`emmc` callers at least fail loudly instead of corrupting. The exFAT path is unaffected (our own formatter, 32-bit FAT length, validated).

The safe boundary, exactly: truncation begins when totalSectors exceeds ~537,133,000 (≈256.1GiB); 256GiB even (536,870,912 sectors) is within range.
