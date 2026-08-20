---
# gosd-qvjs
title: internal/diskfmt's mirrored go-diskfs arithmetic has nothing that would catch a version bump silently invalidating it
status: completed
type: task
priority: normal
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-20T05:54:34Z
---

**Severity: Medium.** No bug exists today — this is a latent risk that only
fires on a future `go-diskfs` version bump, which is exactly the kind of
change that's easy to make without realizing what it touches.

## Verified

Two files in `internal/diskfmt` exist specifically to *re-derive and mirror*
`go-diskfs` v1.9.3's own internal arithmetic, rather than to patch or wrap
its public API:

- `fat32selfconsistent.go`'s doc comment (`:1-24`) works through go-diskfs's
  `sectorsPerFAT` formula by hand — "go-diskfs v1.9.3 solves for
  `sectorsPerFAT` with the numerator `4*(totalSectors-32)`..." — and
  states plainly: "The dependency is not patched here (see bean `gosd-e3e3`
  for the one-line upstream fix). Instead every GoSD FAT32 format hands
  go-diskfs a size it lays out correctly..."
- `fat32limit.go` similarly mirrors go-diskfs's `uint16` sector-count
  narrowing in full width, to refuse oversized media before go-diskfs would
  silently write a corrupt filesystem or panic.

Both are pinned to version-specific behavior of a dependency this project
doesn't control (three separate open beans — `gosd-e3e3`, `gosd-8kdm`,
`gosd-mt53` — track getting the real fixes upstream and eventually deleting
this layer). Until then: nothing in this codebase would fail to compile, or
fail a fast test, if a future `go-diskfs` bump changed its internal
`sectorsPerFAT`/sector-count arithmetic. The mirrored formulas would simply
stop matching reality, and the failure mode is exactly the one this code
exists to prevent in the first place — a volume `go-diskfs` writes that
reads back as internally inconsistent — just reintroduced from the opposite
direction.

## Fix direction (not locked)

- At minimum, a comment at the top of both files (and/or a `//go:generate`-
  style reminder in `go.mod`'s `go-diskfs` line) instructing: "bumping
  go-diskfs requires re-deriving `fat32SelfConsistentSectorLimit` and
  `maxFAT32Bytes`'s arithmetic against the new version before trusting this
  file again."
- Stronger: a test that locks in go-diskfs's *actual* sector-count behavior
  for a few known sizes (not just this package's own re-derivation of it),
  so a version bump that changes go-diskfs's formula fails a test here
  rather than silently shipping.
- Best: once `gosd-e3e3`/`gosd-8kdm`/`gosd-mt53` land upstream and this
  mirroring layer can be deleted, this bean is moot — worth linking as a
  reason to prioritize those beans beyond "upstream cleanliness."

## Todos

- [x] Added the go.mod / file-header reminder: a trailing comment on the\n      `go-diskfs` require line, and expanded doc comments in\n      fat32selfconsistent.go and fat32limit.go cross-referencing each other\n      and the new test below.
- [x] Added it: TestFAT32MirroredArithmeticMatchesGoDiskfsRealOutput\n      (fat32limit_test.go) formats real FAT32 volumes across every\n      cluster-size class plus the ceiling, reads the on-disk BPB back, and\n      asserts go-diskfs's real reservedSectors/sectorsPerCluster/sectorsPerFAT\n      match fat32ReservedSectors/fat32SectorsPerCluster/fat32SectorsPerFAT's\n      predictions - not just this package's own re-derivation checked against\n      itself, which the existing TestFormatFAT32WritesAFATThatIndexesEveryClusterItAdvertises\n      already did.
- [x] Cross-referenced from gosd-8kdm and gosd-mt53 (gosd-e3e3 is already\n      completed).

## Summary of Changes

Both fix directions from this bean's "not locked" list were implemented:

- **Reminder comments**: fat32selfconsistent.go and fat32limit.go now each
  cross-reference the other and name the new test below; go.mod's
  `go-diskfs` require line carries a trailing comment pointing at both files.
- **Locked-behavior test**: TestFAT32MirroredArithmeticMatchesGoDiskfsRealOutput
  formats real go-diskfs FAT32 volumes at one size per cluster-size class
  (512B/4KiB/8KiB/16KiB/32KiB clusters) plus the FAT32 ceiling, reads the
  actual on-disk BPB back (reusing the existing readFAT32BPB helper), and
  asserts go-diskfs's real reservedSectors/sectorsPerCluster/sectorsPerFAT
  match what fat32ReservedSectors/fat32SectorsPerCluster/fat32SectorsPerFAT
  predict. This is a materially different check from the existing
  TestFormatFAT32WritesAFATThatIndexesEveryClusterItAdvertises: that test
  only verifies the real output is *self-consistent* (entries >= clusters+2),
  which a go-diskfs version bump could still satisfy with a formula this
  package no longer correctly models (e.g. a wider FATSz32 field removing the
  need for LargestSelfConsistentFAT32Bytes's trim, or a changed cluster-size
  table) - the new test instead pins the exact numbers, so any formula change
  fails it directly rather than only failing if the new formula also happens
  to be broken.
- Cross-referenced from gosd-8kdm and gosd-mt53 (both still open) as
  additional motivation to land the real upstream fixes and delete this
  mirroring layer.
