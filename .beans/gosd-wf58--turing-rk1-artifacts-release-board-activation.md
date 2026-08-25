---
# gosd-wf58
title: 'Turing RK1: artifacts release + board activation'
status: todo
type: task
priority: normal
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-25T11:15:51Z
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
