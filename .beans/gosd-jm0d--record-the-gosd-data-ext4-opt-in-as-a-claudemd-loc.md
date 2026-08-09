---
# gosd-jm0d
title: Record the GOSD-DATA ext4 opt-in as a CLAUDE.md locked decision
status: completed
type: task
priority: normal
created_at: 2026-08-09T09:54:58Z
updated_at: 2026-08-09T09:56:21Z
---

gosd-95yu (PR #242, merged) shipped gosd build --data-filesystem but only updated COMPATIBILITY.md and docs/. The project-wide locked decision belongs in CLAUDE.md alongside the disk/ and emmc/ ext4 decisions, together with the ext4-golden 8 TiB trap that reads as a capability ceiling but isn't.

## Summary of Changes

Added two locked-decision bullets to CLAUDE.md's project-wide section, alongside the existing `disk/` and `emmc/` ext4 decisions:

1. **GOSD-DATA is FAT32 by default; ext4 is an opt-in** — records the flag, why FAT32 remains the default (host readability is the point of the default), the build-time refusals, and the two consequences a future agent will otherwise trip over: that every ext4 data partition (fixed-size included) grows once on first boot, so `dataexpand` runs for any ext4 image; and that the fixed-size path only ever grows-and-marks, never formats, which is why it needs none of `runEXT4`'s `RootHasOtherContent` second opinion. Explicitly frames itself as an amendment to `gosd-lfu0`'s non-goal rather than an overturn.

2. **The ext4 golden's "8 TiB" is not a ceiling** — this one exists because JP read `verifiedGrowthCeilingBytes` as a real limit and proposed capping `--data-size=expand` at it (2026-08-09). The number is both the cap of the REJECTED `resize_inode` design and a build-host artifact; `meta_bg` was chosen precisely to escape it. Also records why the golden is 512MiB (the 128MiB journal can never be resized, so it is a floor on the seed, not a cap on the result). Points at gosd-2ssb for re-proving the real ceiling.

No code change; docs only.
