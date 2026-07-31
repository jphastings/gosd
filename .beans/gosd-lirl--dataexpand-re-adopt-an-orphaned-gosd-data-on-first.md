---
# gosd-lirl
title: 'dataexpand: re-adopt an orphaned GOSD-DATA on first boot after a reflash'
status: todo
type: feature
priority: normal
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T10:25:45Z
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §2). First, derive the data-partition offset from the flashed MBR (partition 1 start + size) and DELETE dataexpand's mirrored dataPartitionStartLBA constant — with per-app boot sizes (design §0.4) a mirror is wrong by construction. Then insert an Inspect between AddKernelPartition and FormatFAT32 in dataexpand.Run: a FAT32 volume labelled GOSD-DATA at that derived offset is adopted (skip format), anything else formats fresh as today. MBR write stays the commit record; power-loss analysis unchanged. Behavioral tests: reflash-then-boot adopts and preserves contents; foreign/blank content still formats; interrupted adoption redoes cleanly. Applies to --data-size=expand images only — the docs bean covers saying so.
