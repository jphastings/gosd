---
# gosd-m70t
title: 'gosd build --boot-size: per-app boot volume size'
status: todo
type: feature
priority: normal
created_at: 2026-07-31T10:25:45Z
updated_at: 2026-07-31T10:32:47Z
blocked_by:
    - gosd-lirl
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §0.4). Parameterize internal/image's boot partition size: --boot-size flag (default 256MiB, today's constant), validated at flag-parse time (min: fits the payload — surface the current raw go-diskfs disk-full failure as an actionable error naming --boot-size; max: sane MBR/FAT32 bounds). The chosen size becomes the app's layout ABI: changing it in a later release erases GOSD-DATA on upgrade (documented; see the design's §2 grow/shrink analysis). Also: print boot-volume usage (payload bytes / size) at the end of every build so developers watch their headroom shrink across releases. Motivating case: Betamin's >1GB boot volume; also unblocks app-slot OTA (gosd-vxal) slot space for large apps.
