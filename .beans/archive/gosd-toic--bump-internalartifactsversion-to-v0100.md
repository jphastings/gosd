---
# gosd-toic
title: Bump internal/artifacts.Version to v0.10.0
status: completed
type: task
priority: normal
created_at: 2026-08-08T00:19:55Z
updated_at: 2026-08-08T03:45:53Z
---

Follow-up to the artifacts/v0.10.0 release (tagged and pushed by JP 2026-08-08; Build artifacts run 31228292901 succeeded, 9 assets). The release carries: Pi-family CONFIG_EXT4_FS=y (gosd-19kw, PR #223), the radxa-zero-3e/nanopi-zero2 exFAT fragments' first published build (flips [^exfat-soon]), the cubie-a5e board's first fleet rebuild since activation, and the ingress/CA work's kernels unchanged. NOT included: gosd-10fn's fleet-wide btrfs trim (decided after this tag's content merged; rides the next fleet rebuild).

Per docs/artifacts.md steps 5-6: bump Version, flip the COMPATIBILITY.md cells this release unlocks, and verify three ways recorded here:

[x] Clean-machine build: fresh HOME, no --board/--artifacts-dir — ALL SEVEN public boards (cubie-a5e's first default-set build) built from a real download of v0.10.0 (2026-08-08)
[x] Offline re-run: HTTP(S)_PROXY=http://127.0.0.1:9, same fresh HOME — all seven boards rebuilt entirely from cache, zero network
[x] Content spot-check: all three Pi boards' released kernel.config files carry CONFIG_EXT4_FS=y, and pi-zero-2w's kernel8.img binary contains ext4 driver strings (35 matches) — the driver is compiled in, not just configured
[x] COMPATIBILITY.md: pi-ext4 cells ❌→✅ with footnote reworded (shipped in v0.10.0, on-device spot-check rides the next bench pass); all four exfat-soon 🚧 cells (attached-disk + eMMC rows, radxa/nanopi) → ✅ and the now-unused footnote removed; emmc-ext4 rows needed no change
[x] gosd-19kw bean → completed (published in v0.10.0; on-device use verifies at bench)

CORRECTION (2026-08-08, post-merge review): the 'NOT included: gosd-10fn btrfs trim' line above is wrong — PR #222 merged BEFORE the tag; its commit is an ancestor of artifacts/v0.10.0 and the released qemu-virt kernel.config contains no BTRFS_FS. The trim shipped in this release; gosd-10fn is closed accordingly.
