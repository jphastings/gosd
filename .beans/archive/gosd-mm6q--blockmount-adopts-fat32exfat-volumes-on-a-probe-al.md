---
# gosd-mm6q
title: 'blockmount adopts FAT32/exFAT volumes on a probe alone: no sync, no marker, no crash-ordering'
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:13:59Z
updated_at: 2026-08-20T06:04:25Z
---

**Severity: High.** Silent data loss or a permanently wedged volume, in the
public `disk`/`emmc` API, and it violates a locked project rule.

CLAUDE.md: *"Anything that formats, adopts, or commits on-disk state needs an
explicit crash-ordering argument... A filesystem probe is never proof a write
completed — an interrupted format can leave probe-passing debris. The pattern
that survives review is write -> sync -> marker -> sync."*

## Verified — ext4 is protected, FAT32/exFAT are not

`internal/blockmount/blockmount.go:289-291` branches to `runEXT4` only when
`fs == diskfmt.EXT4`. `runEXT4` (`:337-447`) does Format -> SyncDevice ->
Mount -> Grow -> EstablishMarker and gates later adoption on
`EXT4EstablishedMarker`.

Everything else falls through to `:293-312`:

```go
if format {
    ... MountedSources recheck ...
    if err := s.Deps.Format(device, label, fs); err != nil { ... }
}
if err := s.Deps.Mount(device, mountpoint, fs); err != nil { ... }
return device, nil
```

No `SyncDevice`. No marker. And the adoption gate at `:259-262` is
`labelMatches(contents, label) && contents.FS == fs` — a pure probe, exactly
what the rule forbids.

`SyncDevice` is already wired as a dependency in both
`disk/platform_linux.go` and `emmc/platform_linux.go`. It is simply never
called on this path.

## Why the probe is not safe here — the project already documented this

`cmd/gosd-init/internal/dataexpand/dataexpand.go:117-124`, in this repo's
own words:

> "go-diskfs writes the boot sector, FATs, root directory and finally the
> label with no sync between them, so a power cut mid-format can leave a
> volume that inspects as a correctly labelled FAT32 filesystem over
> incomplete FAT tables."

`internal/diskfmt/exfatformat.go:94-145` (`writeExFAT`) has the same shape:
boot region -> FAT -> bitmap -> up-case table -> root directory (label)
last, unsynced. The label — the thing `labelMatches` tests — is the last
thing written in both.

dataexpand mitigates this for its own partition (`:357-424`). blockmount
mitigates it for ext4. Nothing mitigates it for FAT32/exFAT, which is the
**documented universal default** for removable media (`disk/disk.go:143`).

## Attack / failure

App calls `disk.FormatAndMount("APPDATA", "/storage", false)` with
`Options{Filesystem: disk.FAT32}` on a blank USB disk. Power is cut during
`Format()`, or after it returns but before writeback reaches the medium —
nothing on this path ever fsyncs. Two outcomes:

1. **Label did not land** (likeliest — written last): next boot `Inspect`
   reports FAT32 with a non-matching label, `contents.Blank` is false, and
   `!destructive` returns `ErrRefusedFormat` — **forever**, on every boot,
   until a human passes `destructive=true`. Storage permanently wedged
   despite never having held data.
2. **Label landed, FATs torn**: `labelMatches` succeeds, so `Run` mounts it
   as "already provisioned". vfat's mount-time validation is shallow, so the
   app gets a volume with corrupt cluster chains — silent corruption on
   first write, because nothing ever checked that the format finished.

`internal/blockmount/blockmount_test.go`'s crash/establishment tests
(`TestRunEXT4FreshFormatEstablishesInOrder`,
`TestRunEXT4CrashDebrisWithNoMarkerReformats`, and siblings, `:482-903`) are
all ext4-only — there is no FAT32/exFAT equivalent, so this is untested as
well as unimplemented.

## Fix

Minimum: call `s.Deps.SyncDevice(device)` after `Format()` succeeds in the
non-ext4 branch, before `Mount()`.

Parity: after the sync, write a reserved marker with the existing
`diskfmt.CreateEmptyFile`, sync again, and gate the "already provisioned ->
mount only" branch on that marker for FAT32/exFAT the same way `runEXT4`
gates on `EXT4EstablishedMarker`. `RootFileExists`/`CreateEmptyFile` are
already used by dataexpand for exactly this.

Mind `diskfmt.CreateEmptyFile`'s documented caveat that leading-dot names
are invisible to go-diskfs's own directory listings when picking the marker
name.

## Todos

- [x] `SyncDevice` after Format on the FAT32/exFAT path
- [x] Establishment marker for FAT32/exFAT, written and synced after the format
- [x] Gate the adopt-without-format branch on the marker, not on the label probe
- [x] Port `runEXT4`'s crash-debris tests to FAT32 and exFAT
- [x] State the crash-ordering argument for these two filesystems in blockmount's package doc, which currently only argues it for ext4


## Summary of Changes

`runEXT4` is now `establish`, and every filesystem goes through it. The
FAT32/exFAT tail of `Run` (format-then-mount, no sync, no marker) is gone.

**The establishment sequence, all three filesystems:** `Format` → `SyncDevice`
→ `Mount` → `Grow` (ext4 only) → `EstablishMarker`. Adoption of an existing
volume is gated on the marker, never on the label probe.

**Crash-ordering argument (FAT32/exFAT).** What is provably durable before the
commit record lands:

1. `Format` writes the whole filesystem to the raw device. `diskfmt` fsyncs
   nothing mid-format — `eraseLeadingRegion`'s doc states that policy
   outright. So nothing is durable here, and no *ordering* among the
   formatter's own writes is durable either: writeback order is not program
   order, so an arbitrary SUBSET may be on the medium.
2. `SyncDevice` fsyncs the device node. When it returns, every byte `Format`
   wrote is on the medium. (Both `diskfmt` and this call address the same
   block device inode, so they share one page cache; the fsync covers writes
   issued through `diskfmt`'s own, already-closed, descriptor.) This is the
   step the FAT path never had.
3. `Mount` interprets only what step 2 made durable.
4. `EstablishMarker` writes `.gosd-established` and — on FAT32/exFAT —
   `syncfs(2)`s the mount. It is the commit record, written only after every
   step above returned.

What an interruption leaves, and what the next boot does:

- **Before the boot sector lands:** FAT32's formatter erases the whole 1MiB
  window `Inspect` probes before writing into it, and exFAT's rewrites all of
  it, so the device reads `Contents{Blank: true}` → formatted freely, nothing
  lost.
- **Boot sector landed, label did not:** go-diskfs reads a FAT32 label from
  the *root directory* (`fat12.Label()` scans for the volume-label entry), and
  `Create` writes it last, after zeroing the root cluster. So the volume reads
  as FAT32 under an empty/wrong label → foreign content → refused without
  `destructive`. Unchanged by this bean, and deliberately: nothing
  distinguishes it from a stranger's freshly-formatted stick, so GoSD keeps
  refusing rather than wiping. (This is the "wedged forever" outcome the bean
  described; it is inherent, not fixable by a marker. Recorded below as
  considered-and-declined.)
- **Label landed, rest torn:** label + filesystem match, no marker. The marker
  settles it: no marker means the mountpoint was never handed to an app, so
  nothing here is worth keeping → unmount, reformat, establish. No consent
  needed.
- **After the marker lands:** established, because the marker is durable only
  once everything before it was.

**Compatibility — no deployed card with data is reformatted.** A pre-marker
FAT32/exFAT volume cannot be distinguished from crash debris with certainty
(both read "my label, my filesystem, no marker"), so the two are separated on
the one signal that does differ: the root directory. Neither formatter creates
a file (go-diskfs zeroes FAT32's root cluster before writing the label; exFAT's
root holds only label/bitmap/up-case entries, none of which the kernel lists),
so a file in the root can only have been written through a mount an app was
handed — which only happens after a format completed. Therefore:

- no marker + **files in the root** → **adopt**, and write the marker in
  passing (one-time, in-place upgrade; best-effort, so a full or
  read-only-remounted volume still mounts exactly as before);
- no marker + **empty root** → reformat. This is the ONLY state a deployed
  card can be reformatted in, and it holds no files by definition.

The net direction is one-way: every state this code adopts was already adopted
before (the old code adopted on the label alone), so nothing that used to be
adopted stops being adopted except the provably-empty case.

The deliberate asymmetry — the same state REFUSES on ext4 — is argued in
`establish`'s doc: ext4 has a step after the format that nothing else records
(the one-time grow), and no legacy population, since its marker shipped with
its default.

**Other deliberate choices:**

- `EstablishMarker` takes the filesystem. ext4 keeps fsync(file) + fsync(dir);
  FAT32/exFAT get `syncfs(2)`. Not cosmetic: FAT has no journal and no
  ordering between a directory entry, the FAT chain it points into and the
  free-space accounting, so there is no per-file fsync that promises a
  consistent subset — and a directory fsync's availability is a per-driver
  detail, not a VFS guarantee, so relying on one would have risked a hard
  failure on every real-hardware FAT32 format, in a path no CI job exercises.
- `RootHasOtherContent` takes the filesystem too: `lost+found` is reserved on
  ext4 only. Treating it as reserved on FAT would have made a FAT volume whose
  only entry is a `lost+found` directory look empty, i.e. reformattable.
- `EXT4EstablishedMarker`/`EstablishEXT4Marker`/`EXT4MarkerEstablished` are
  renamed without the EXT4 prefix; they are no longer ext4-only. `dataexpand`
  updated to match.
- One genuinely new destructive path: an unmountable volume under the app's own
  label is now reformatted under `destructive: true` (and refused with
  `ErrRefusedFormat` + `ErrUnmountable` without it) instead of returning a bare
  mount error. Matches ext4 since gosd-psj0. Called out in the change file.

**Adversarial review pass — what was attacked:**

1. *Does "content ⇒ adopt" let torn debris be adopted?* Yes, in one narrow
   case: a reformat over a volume that already had content, crashed such that
   stale root-directory pages survived while the label page landed. It needs
   `destructive: true` (consent to destroy that content), and the old code
   adopted the same state unconditionally — strictly narrower than before.
2. *Can a volume hold data with an empty root?* Not through a filesystem: an
   unreferenced file is unreachable. Raw writes beneath the filesystem by an
   app that also calls `FormatAndMount` for the same device would qualify —
   contrived, and previously that volume was mounted (not reformatted) too.
3. *Does the kernel list anything in a fresh FAT/exFAT root?* If it did, a
   fresh volume would look used → adopt instead of repair. Wrong-guess
   direction is the safe one; documented.
4. *Can the kernel hide a real file?* Only 0xE5-deleted entries, which are not
   files.
5. *Directory fsync on vfat/exfat* — designed out with syncfs (see above).
6. *Does `SyncDevice` cover writes made through `diskfmt`'s own descriptor?*
   Yes — one page cache per block-device inode; the same argument ext4 already
   relies on.
7. *Does any of this change ext4's call sequence?* No: the untouched ext4
   ordering tests still pass byte-for-byte.
8. *`Deps.Grow` nil for FAT callers* — the grow is now explicitly gated on
   `fs == diskfmt.EXT4`, pinned by `TestRunNeverGrowsAnythingButEXT4`.
9. *Marker-name collision with dataexpand's* — different devices, and
   different names on FAT (`gosd-data-established` vs `.gosd-established`).
10. *Race between the marker check and the reformat* — `runMu` is held and the
    mountpoint has not been returned.
11. *New failure mode:* a volume whose root cannot be read at all (I/O error)
    with no marker now errors instead of mounting. Refusing to guess is the
    house rule; noted rather than special-cased.
12. *`diskfmt.RootFileExists` cannot see a leading-dot name* — so it can never
    be used to look for this marker on FAT32. Nothing does; documented on the
    constant.

**Not done, deliberately:** the "label never landed" wedge (bullet 2 of the
interruption list) is unfixable without letting GoSD wipe an unlabelled
stranger's volume without consent. Worth a follow-up bean only if it is ever
seen in the field.

**Tests:** `TestRunFAT*` in `internal/blockmount` mirror the ext4
establishment/adoption/crash-debris set across FAT32 and exFAT (fresh-format
ordering, adoption, debris repair, legacy adoption + marking, legacy adoption
surviving a failed marker write, legacy adoption unaffected by `destructive`,
mount-failure both ways, sync-failure stopping before the marker, never
growing). `TestRunEXT4NoMarkerButRealContentStillRefuses` pins the asymmetry.
`emmc` gains wiring tests proving its deps reach the shared sequence.
