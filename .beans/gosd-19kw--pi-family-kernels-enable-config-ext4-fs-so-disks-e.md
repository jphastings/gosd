---
# gosd-19kw
title: 'Pi-family kernels: enable CONFIG_EXT4_FS so disk/''s ext4 default works on Pi USB drives'
status: todo
type: feature
priority: normal
created_at: 2026-08-07T19:11:14Z
updated_at: 2026-08-07T19:11:14Z
parent: gosd-lfu0
---

JP (2026-08-07): fleet featureset should be complete — the Pi boards are the only ones whose kernels lack ext4 (EXT4_FS not set in all three recorded kernel.configs), so disk/'s ext4-by-default fails its /proc/filesystems preflight there (USB drives are the affected surface; Pis have no eMMC/NVMe).

- Add CONFIG_EXT4_FS=y (+ whatever JBD2/crc dependencies olddefconfig doesn't chain automatically — verify from the build) to all THREE Pi fragments (pi-zero-2w, pi-zero-w, pi-3b), family-wide per the per-family pin convention. RequiredY derives from the fragment for Pi boards, so the fragment line is the assertion.
- Audit the resulting kernel.configs per the Pi-defconfig-trap rule.
- **Artifacts dance**: ship the fragment change WITHOUT bumping artifacts.Version; coordinate the release tag with gosd-10fn's btrfs trim so both kernel changes ride ONE artifacts release (they touch disjoint board families — Pi here, mainline fleet there — so one tag covers all seven boards' rebuilds). CI workflow_dispatch pre-merge run is the build verification; a size check on the Pi kernels (ext4+jbd2 adds ~1MiB) confirms the change landed.
- COMPATIBILITY.md ext4 rows for the Pi boards flip once the Version bump lands.
