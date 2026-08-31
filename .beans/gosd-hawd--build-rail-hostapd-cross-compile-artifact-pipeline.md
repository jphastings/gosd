---
# gosd-hawd
title: 'Build rail: hostapd cross-compile (arm64 + armv6) + artifact pipeline'
status: todo
type: task
priority: normal
created_at: 2026-08-31T05:36:11Z
updated_at: 2026-08-31T06:40:17Z
parent: gosd-qfbk
---

WiFi AP epic gosd-qfbk bean 1. No dependency on the other child beans; blocks
bean 4 (build-time wiring needs the real artifact shape) and bean 6 (bench
verification needs the real binary).

## Locked decisions

- New arch-keyed build spec (mirrors `internal/kernelspec`'s declarative
  shape, but keyed on GOARCH not board — hostapd's build depends only on
  target arch, not per-board kernel config, so it's simpler than a kernel
  build: "compile once per GOARCH").
- Docker/Podman cross-compile via `internal/container`'s existing daemon-
  driving code, musl static build (per `docs/externals.md`'s own "prefer
  musl over glibc for anything nontrivial" guidance).
- Verify the result with `internal/staticelf.Verify` — the same check
  `--with-external`'s build already runs.
- Ships as one more file inside each applicable board's **existing**
  `internal/artifacts` per-board tarball, reusing `manifest.json`/
  `ManifestSHA256`, `EnsureBoard`'s download/cache/verify, and the existing
  tag-first-bump-second `artifacts/vX.Y.Z` release procedure — NOT a new
  release channel and NOT the pinned-URL-and-sha256 third-party-blob
  pattern (no prebuilt static hostapd exists for these targets the way
  GPU/WiFi firmware does).
- **BOTH arm64 and armv6** (epic decision 5, revised 2026-08-31 — pi-zero-w
  is in scope). Two cross-compiles, not one. armv6 is the riskier leg and is
  ordinary build verification, not an architectural unknown: hostapd ran on
  armv6 Pis for years. If the armv6 cross-compile turns out to be genuinely
  intractable, that is a finding to report — not a reason to silently ship
  arm64 only.

## Licensing — hostapd is BSD-3-clause, NOT GPL (corrected 2026-08-31)

An earlier version of this bean said "GPL-2 provenance ... same pattern
already proven for the kernel". **That was wrong.** hostapd/wpa_supplicant
were dual BSD/GPLv2 historically, but upstream **removed the GPLv2 option on
2012-02-11**; everything since is BSD-3-clause only (upstream `COPYING`:
"...any distribution of this software after February 11, 2012 is no longer
under the GPL v2 option"). Files that still carry GPLv2 pointers keep them
"only for attribution purposes".

So the obligations here are attribution-only: reproduce the copyright
notice, license text and disclaimer alongside the binary. No source
disclosure, no copyleft, nothing touching GoSD's or an app's own licensing.
Keep recording source repo/commit/build config anyway — it's useful for
reproducibility, it just isn't GPL provenance.

**A separate, real obligation to actually check in this bean:** hostapd's
`nl80211` driver normally links **`libnl`, which is LGPL-2.1**. A fully
static musl build (the plan above) statically links it, and LGPL-2.1 §6 then
requires that recipients be able to relink against a modified libnl — i.e.
ship relinkable objects, or libnl's complete corresponding source and build
scripts. Check whether a no-`libnl` hostapd build is viable at the pinned
version first; if it isn't, handle libnl's LGPL notice and relink
obligation separately from hostapd's own BSD notice. Full report:
scratchpad `gpl-license-report.md` (2026-08-31), not committed.

## Todos

- [ ] Arch-keyed hostapd build spec + Docker recipe (musl static)
- [ ] arm64 cross-compile proven
- [ ] armv6 cross-compile proven (report honestly if intractable)
- [ ] Determine whether hostapd can build without `libnl` at the pinned
      version; if not, record and satisfy libnl's LGPL-2.1 §6 relink
      obligation
- [ ] Bundle hostapd's BSD-3-clause license text with the artifact
- [ ] `internal/staticelf.Verify` wired into the build/CI job (both arches)
- [ ] Fold into the existing per-board `internal/artifacts` manifest/tarball
- [ ] Source repo/commit/config recorded in `manifest.json` (reproducibility,
      not GPL provenance)
- [ ] Ship in the next `artifacts/vX.Y.Z` release per the tag-first,
      bump-second procedure (docs/artifacts.md) — do NOT bump
      `internal/artifacts.Version` in this PR
- [ ] Pre-merge-test the artifacts CI job via `workflow_dispatch` on the PR
      branch before the tag exists
