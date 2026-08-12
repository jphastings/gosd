---
# gosd-mm6q
title: 'blockmount adopts FAT32/exFAT volumes on a probe alone: no sync, no marker, no crash-ordering'
status: todo
type: bug
created_at: 2026-08-12T04:13:59Z
updated_at: 2026-08-12T04:13:59Z
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

- [ ] `SyncDevice` after Format on the FAT32/exFAT path
- [ ] Establishment marker for FAT32/exFAT, written and synced after the format
- [ ] Gate the adopt-without-format branch on the marker, not on the label probe
- [ ] Port `runEXT4`'s crash-debris tests to FAT32 and exFAT
- [ ] State the crash-ordering argument for these two filesystems in blockmount's package doc, which currently only argues it for ext4
