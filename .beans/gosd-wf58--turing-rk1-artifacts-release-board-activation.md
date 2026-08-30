---
# gosd-wf58
title: 'Turing RK1: artifacts release + board activation'
status: in-progress
type: task
priority: normal
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-30T20:27:49Z
parent: gosd-bntd
blocked_by:
    - gosd-phjh
    - gosd-bib8
---

Ship the kernel + U-Boot artifacts in a new artifacts/vX.Y.Z release (tag-first, per CLAUDE.md's artifact-release rule -- do NOT bump internal/artifacts.Version in the same PR that adds the build-source pins). Once that release is published, a follow-up PR flips turing-rk1 from RegisterInternal to Register, bumps internal/artifacts.Version + ManifestSHA256, adds the fixture to cmd/gosd/testdata/fake-artifacts/, and adds the COMPATIBILITY.md bring-up row (internal/repocheck enforces both mechanically). Mirrors gosd-zh95 (cubie-a5e) / gosd-h8a8 (rock-4se).



## Important: catalog generation exclusion needs a real fix at activation time

`cmd/gosd/build.go`'s `writeCatalog` currently excludes turing-rk1 from
`--publish-catalog` output via `boards.IsInternal(b.Name())` -- correct for
now, but that check is about registration status, not hardware capability.
Flipping `RegisterInternal` -> `Register` (the mechanical step this bean
does for every other board) would make turing-rk1 suddenly PASS that check
and start getting a broken Imager catalog entry, even though the board can
never actually be flashed via Imager (no SD/microSD slot at all -- see the
epic). This bean needs an explicit, permanent exclusion for turing-rk1 from
catalog generation that survives the public flip -- not just the
IsInternal check every other board activation relies on. Consider a
board-level capability (e.g. a CatalogSupport()-shaped method mirroring
UsbGadgetSupport/EXT4Support) rather than a hardcoded ID check in
writeCatalog, since any future board without a card slot would hit the
same trap.


## Correction (2026-08-30, JP): the catalog-exclusion concern doesn't apply

The "Important" section above worried that flipping `RegisterInternal` ->
`Register` would make turing-rk1 start appearing in the Raspberry Pi
Imager catalog (`--publish-catalog`) even though it has no SD/microSD
slot to flash via a normal card reader. JP's correction: Raspberry Pi
Imager can also write directly to the RK1's eMMC as exposed through the
Turing Pi 2 (a mounted block device, the same class of path Imager
already writes any USB-attached target through) -- so a catalog entry is
genuinely usable for this board, not a broken promise. No
CatalogSupport()-shaped exclusion mechanism is needed: `RegisterInternal`
-> `Register` can proceed exactly like every other board's activation,
with no special-casing. Superseding the concern raised above rather than
deleting it, per CLAUDE.md's "stop and say so" rule for locked decisions
that prove wrong in practice.

## Progress: PR 1 of 2 (tag-first)

Wired `dist/turing-rk1.tar.zst` into `build-artifacts.yml`'s real release
upload list, and added an `artifacts: minor` change file so the next
knope release PR merge actually cuts a new `artifacts/vX.Y.Z` tag
carrying it. Board stays `RegisterInternal` through this PR -- no
CLI-visible change yet. PR 2 (bump-second) follows once that release is
confirmed live: flip to `Register`, bump `internal/artifacts.Version` +
`ManifestSHA256`, add the `cmd/gosd/testdata/fake-artifacts/` fixtures,
add the COMPATIBILITY.md bring-up row, update CLAUDE.md's Board IDs entry
to drop the "internal only until..." caveat.
