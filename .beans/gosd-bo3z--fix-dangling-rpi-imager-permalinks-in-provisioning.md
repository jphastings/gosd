---
# gosd-bo3z
title: Fix dangling rpi-imager permalinks in provisioning-formats.md
status: completed
type: task
priority: low
created_at: 2026-07-06T00:34:45Z
updated_at: 2026-07-06T02:26:21Z
---

PR #28 discovered the pinned rpi-imager commit 204a6eee... cited throughout docs/provisioning-formats.md no longer resolves on GitHub (likely a rebased/deleted ref). Tag v2.0.10 resolves to commit 467be3d3... with byte-identical content for the files we checked. Rewrite the ~30 permalinks to the v2.0.10 tag commit, spot-checking each cited line number still matches (line numbers can shift between the dangling commit and the tag). internal/catalog/testdata/README.md is already fixed — this bean is only the research doc.


## Summary of Changes

Rewrote every `204a6eee47c2c46da453d4de4138f08619a8c0e6` reference in
`docs/provisioning-formats.md` to `467be3d3e88f5d83fa78c78788f6e6fdce61a47e`
(the commit tag `v2.0.10` resolves to). 33 occurrences total: 1 prose mention
of the commit hash, plus 32 permalinks (28 with `#Lnn-Lmm` line anchors, 4
bare file links with no anchor).

Verification method: cloned `raspberrypi/rpi-imager` and checked out
`467be3d3e88f5d83fa78c78788f6e6fdce61a47e` directly (confirmed via `git tag
--points-at HEAD` → `v2.0.10`), then read every cited line range out of the
real checkout and compared it against the surrounding doc text's claims.

Result: **all 28 line-anchored links matched exactly — zero line-number
adjustments were needed.** All 4 anchor-less file-path links resolve too.
This confirms the bean's premise that content is byte-identical between the
dangling commit and v2.0.10, at least for every file this doc cites. No claim
in the doc failed verification, so no editor's notes were added to
`docs/provisioning-formats.md`.

One related discrepancy found but left untouched (out of this bean's scope,
which is `docs/provisioning-formats.md` only): the bean body's premise that
`internal/catalog/testdata/README.md` "is already fixed" is incorrect — that
file still cites the dangling `204a6eee...` commit in two links (lines 6 and
9). Worth a quick follow-up fix since it has the same dangling-permalink
problem this bean was created to solve.
