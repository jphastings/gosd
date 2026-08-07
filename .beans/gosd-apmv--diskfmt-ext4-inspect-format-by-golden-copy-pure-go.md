---
# gosd-apmv
title: 'diskfmt: ext4 inspect + format-by-golden-copy (pure Go)'
status: todo
type: task
priority: normal
created_at: 2026-08-07T09:57:52Z
updated_at: 2026-08-07T09:58:28Z
parent: gosd-lfu0
blocked_by:
    - gosd-u988
---

internal/diskfmt grows ext4 support alongside FAT32/exFAT for epic gosd-lfu0: Inspect (recognize an ext4 superblock, read its 16-byte volume label + UUID) and Format (stream the decompressed golden image from gosd-lfu0's checked-in asset to the target, then set label + fresh random UUID in the superblock and recompute the superblock checksum — csum_seed makes that superblock-local; verify). Pure Go, fake/file-backed tests that pass on macOS, mirroring diskfmt's existing structure and its documented go-diskfs-distrust conventions (not applicable to ext4 — we own this path end to end).

## Todos

- [ ] Superblock reader: magic, feature flags (fail loudly on unknown incompat features), label, UUID, block count
- [ ] Format: golden copy + label (16-byte limit enforced with an actionable error) + random UUID + superblock csum update
- [ ] Behavioral tests: format a file-backed target, re-Inspect it, assert label/UUID/features; corrupt-superblock and truncated-golden-write cases produce honest errors/probe failures
- [ ] Document (docstrings) that Format's output is only established once blockmount's marker lands — a bare probe-passing superblock is expected debris after a crash
- [ ] Quality gates + PR
