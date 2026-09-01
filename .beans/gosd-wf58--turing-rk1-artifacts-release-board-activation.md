---
# gosd-wf58
title: 'Turing RK1: artifacts release + board activation'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-31T12:05:32Z
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


## PR 1 merged, release confirmed live (2026-08-31) — starting PR 2

PR #380 merged; the subsequent knope release PR also merged, cutting
`artifacts/v0.10.4` (published 2026-08-31T10:28:50Z) with
`turing-rk1.tar.zst` confirmed attached alongside every other board's
tarball. Ready for PR 2 (bump-second): the actual activation.

**PR 2 checklist** (mirrors gosd-zh95/gosd-h8a8's shape exactly):
1. `internal/boardset/boardset.go`: flip
   `boards.RegisterInternal(turingrk1.New())` -> `boards.Register(...)`.
2. `internal/artifacts/artifacts.go`: bump `Version` to `v0.10.4` and
   `ManifestSHA256` to the sha256 of that release's `manifest.json`
   (`build/artifacts/pin-bump.sh` writes both together — use it, don't
   hand-edit).
3. Add `cmd/gosd/testdata/fake-artifacts/` fixtures for turing-rk1 (check
   cubie-a5e's PR #205 for the pattern: dummy files matching the real
   artifact names — kernel Image, its DTB, idbloader.img, u-boot.itb).
4. Add the COMPATIBILITY.md bring-up row + feature-table column for
   turing-rk1 (internal/repocheck's TestCompatibilityBringUpRows enforces
   this once public).
5. CLAUDE.md: drop the "internal only until its artifacts release lands"
   caveat from the turing-rk1 Board IDs entry.
6. Verify per CLAUDE.md's three-way artifact-bump rule: clean-machine
   build (fresh HOME, no --board/--artifacts-dir), offline re-run (dead
   proxy, succeeds from cache), content spot-check (e.g. dtc -I dtb -O
   dts on the released DTB, or similar, confirming real compiled content
   not a stub).
7. No catalog-exclusion special-casing needed (see the correction above)
   — RegisterInternal -> Register is the whole registration change.
8. Branch: bean/gosd-wf58-turing-rk1-activation (PR 2 of 2). Needs its
   own gosd:/artifacts: change file (this IS user-facing: gosd build
   --board turing-rk1 becomes selectable for the first time) — no
   "no release notes" label.


## PR 2 complete, activation done (2026-08-31)

`RegisterInternal` -> `Register` flipped, `internal/artifacts.Version`
bumped to v0.10.4 (`ManifestSHA256` via `build/artifacts/pin-bump.sh`,
not hand-edited), the missing `rk3588-turing-rk1.dtb` fixture added
(the other three artifact names already existed as shared fixtures),
COMPATIBILITY.md gained the bring-up row, a full feature-table column,
two new footnotes (`rk1-gadget`, `rk1-audio`), and a board note (no
SD/microSD slot, flashing guide pointer, GPIO out of scope). CLAUDE.md's
"internal only until..." caveat dropped from the Board IDs entry. No
catalog-exclusion special-casing needed, per the correction above.

Three-way artifact-bump verification (CLAUDE.md's rule), all passed:
- Clean-machine build: fresh HOME, `--board turing-rk1`, no
  `--artifacts-dir` -> real download from artifacts/v0.10.4 succeeded.
- Offline re-run: same fresh HOME, dead proxy (`HTTP(S)_PROXY=
  http://127.0.0.1:1`) -> succeeded entirely from cache, no network hit.
- Content spot-check: `dtc -I dtb -O dts` on the downloaded DTB shows
  `compatible = "turing,rk1", "rockchip,rk3588"`, `model = "Turing
  Machines RK1"` -- genuine compiled content, not a stub. `u-boot.itb`
  (1358336 bytes, matches the original bring-up's recorded size) is a
  valid FIT/DTB. `Image` is 67MB, `kernel.config` 270KB -- both real.

All quality gates green: go build/vet/test, gofmt, golangci-lint (native
+ GOOS=linux).

turing-rk1 is now a fully public board.
