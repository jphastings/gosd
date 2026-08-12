---
# gosd-f83b
title: 'diskfmt: a FAT32 label with a space at byte 8 silently loses it on Inspect — narrower round-trip gap survives gosd-xq9l''s edge-space fix'
status: completed
type: bug
priority: high
created_at: 2026-07-31T08:00:00Z
updated_at: 2026-08-01T00:00:00Z
---

Found while implementing `gosd-xq9l` (blockmount label round-trip fix),
verified empirically with `diskfmt.Format`/`diskfmt.Inspect` round trips on
real (file-backed) FAT32 and exFAT volumes.

`gosd-xq9l` proved (and fixed) that a label with a *leading or trailing*
space cannot round-trip. Its round-trip test deliberately covers only
representative admitted classes and does not claim full coverage of every
interior-space position — this bean is that missing coverage's finding: an
*interior* space can also fail to round-trip on FAT32, for a narrower reason
than edge padding.

**Mechanism:** go-diskfs's generic directory-entry parser
(`fat12.parseDirEntries`) splits every 11-byte 8.3 name into an 8-byte
short-name field and a 3-byte extension field, and trims trailing spaces off
*each independently* before `fs.Label()` concatenates them
(`filenameShort + fileExtension`). A space that lands exactly at byte 8 (the
last byte of the short-name field) is indistinguishable, to that per-field
trim, from padding — so it is dropped, even though it is an interior
character of the label as a whole, not an edge character.

Confirmed empirically (FAT32 only — exFAT stores/reads the label as one
contiguous UTF-16 run, no 8.3 split, and round-trips all of the same cases
correctly):

| label (9 chars) | FAT32 Inspect returns |
| --- | --- |
| `"ABCDEFG H"` (space at byte 8) | `"ABCDEFGH"` — the space vanishes |
| `"A B C D E"` (spaces at 2,4,6,8) | `"A B C DE"` — only the byte-8 space vanishes |

Labels with an interior space *not* at that boundary (e.g. `"AB CD"`) round
-trip correctly — this is a narrow, position-specific gap, not a general
"interior spaces are unsafe" finding.

**Impact:** identical failure class and consequence to gosd-xq9l — Run's
idempotency comparison (`labelMatches`) never matches a label of this exact
shape once it has been formatted as FAT32, so every subsequent boot either
silently reformats and destroys the app's persistent data
(`destructive=true`) or permanently refuses with `ErrRefusedFormat`
(`destructive=false`). `emmc` is FAT32-only by design, so it is always
exposed; `disk` is only exposed when it defaults to or is asked for FAT32
(`Options.Filesystem` can pick exFAT instead, which is immune).

**Fix directions to evaluate** (not yet designed):
- go-diskfs's boot-sector parsing (`dos40ebpb.go`) separately holds a
  `VolumeLabel` copy trimmed as *one* 11-byte string
  (`re.ReplaceAllString(string(b[32:43]), "")`, no 8/3 split) — reading the
  label from there instead of (or as a cross-check against) `fs.Label()`'s
  directory-entry path may sidestep the bug entirely; needs verifying it is
  kept in sync by `SetLabel`/`SetRootDirLabel` and by every code path that
  can change the label after creation.
  - `internal/diskfmt/diskfmt.go`'s `inspectFAT` (~line 147) is the read
    site; `trimLabel` (~line 168) is the shared trim helper.
- Failing that, a diskfmt-side write-time fix: never let the two 8.3
  sub-fields split *inside* a run of label content — e.g. detect when a
  requested label would place a space at byte 8 and lay the bytes out
  differently while keeping the same 11-byte on-disk label.
- As a last-resort, narrower belt-and-braces: `blockmount.ValidateLabel`
  could reject labels with a space at that specific FAT-8.3 byte-8 boundary
  — this couples general validation to a FAT32 implementation quirk exFAT
  doesn't share, so prefer a diskfmt-side fix if one is practical.

Whichever fix is chosen, extend
`internal/blockmount.TestAdmittedLabelsRoundTripToWhatRunCompares` (added by
gosd-xq9l) with a `"ABCDEFG H"`-shaped case so the invariant it pins covers
this position too.

## Summary of Changes

**Chosen fix: `blockmount.ValidateLabel` rejection (the bean's third,
last-resort direction), matching gosd-xq9l's own pattern rather than a
diskfmt-side or go-diskfs-side change.** The other two directions this bean
sketched were re-examined and set aside: reading the boot-sector
`VolumeLabel` copy instead of (or as a cross-check against) `fs.Label()`
would still need proving that copy is kept in sync by every label-writing
path (`SetLabel`, `SetRootDirLabel`, and any future one) — a *bigger*
verification burden than the exhaustive test below, for a fix that only
protects FAT32 reads GoSD's own diskfmt package makes, not any other
consumer of a label GoSD wrote. A write-time layout change (never let the
8.3 split fall inside label content) would touch go-diskfs's on-disk
encoding — this is out of scope: **go-diskfs is third-party and was not
modified**, and no PR was opened against it. If a diskfmt-side or upstream
fix is later wanted instead of (or in addition to) the blockmount-side
reject, the concrete upstream change would be: special-case the
volume-label directory entry in `fat12.parseDirEntries`
(`filesystem/fat12/directoryentry.go`) to concatenate the raw 8+3 raw bytes
*before* trimming, rather than trimming `filenameShort` and `fileExtension`
independently and concatenating the trimmed results — volume labels have no
8.3 filename semantics to preserve (they're not files), so nothing else
should depend on the per-field split for that one entry kind. Left here for
JP to weigh, not filed upstream.

**The exact rejected set, derived from `fat12.parseDirEntries` and
`fs.Label()` and confirmed empirically:** a space (0x20) at 0-based byte
index 7 — the short-name field's own last byte — in a label longer than 8
bytes (i.e. 9, 10 or 11 bytes). No other byte position is affected: the
extension field (bytes 8-10) is only ever padded at its own trailing edge
(byte 10, or earlier if the label is shorter), which gosd-xq9l's
leading/trailing-space rule already forbids landing on. This is layered on
top of, not a replacement for, gosd-xq9l's edge-space rule — the two rules
together are exactly `ValidateLabel`'s complete round-trip-safety check.

- `internal/blockmount.ValidateLabel` now also refuses a label with a space
  at byte index 7 when the label is longer than 8 bytes, with an actionable
  message naming the position and the mechanism (FAT's 8-byte name / 3-byte
  extension split, trimmed independently on read-back).
- Belt-and-braces: `labelMatches` now also recognises the exact FAT
  directory-entry round-trip transform (`fatDirectoryEntryRoundTrip`: pad to
  11 bytes, split at the same byte-7 boundary, trim each field
  independently, concatenate) for FAT32 content, alongside the existing
  edge-trim comparison from gosd-xq9l — so a label that somehow bypassed
  both `ValidateLabel` rules still can't provoke Run's reformat-every-boot
  loop. exFAT keeps the original edge-trim comparison only, since it has no
  8.3 split to mirror.
- **Proof, per the bean's ask:** `TestAllPositionsRoundTripOrAreRejected`
  exhaustively builds every label shape from 1 to 11 bytes with a space at
  each byte position (66 shapes), and for each one either asserts
  `ValidateLabel` rejects it or performs a real `diskfmt.Format`→`Inspect`
  round trip on *both* FAT32 and exFAT and asserts the label comes back
  exactly and `labelMatches` recognises it — confirming the rejected set is
  neither too wide (nothing round-trippable is refused) nor too narrow
  (nothing corrupting is admitted), and that exFAT is unaffected at every
  position. `TestValidateLabelRejectsByte7SpaceActionably` and
  `TestValidateLabelAllows8ByteLabelsWithATrailingContentByte` pin the
  actionable-message and length-boundary behaviour directly.
  `emmc`/`disk`'s existing `TestLabelErrorsAreAttributedToThisPackage` each
  got a byte-7 case alongside their trailing-space one.
