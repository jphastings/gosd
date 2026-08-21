---
# gosd-o34r
title: 'diskfmt: a FAT32 format over ext4 leaves the ext4 superblock, so Inspect reports the dead filesystem and gosd-init halts'
status: completed
type: bug
priority: normal
created_at: 2026-08-16T19:10:14Z
updated_at: 2026-08-20T04:32:41Z
---

Found on the Cubie A5E bench (bean gosd-6pfn), but **not board-specific** —
this is `internal/diskfmt` + gosd-init, and it can halt any gosd device.

## What happened on hardware

Boot 1 of a stock `--data-size=expand` image created and formatted the data
partition, mounted it and ran /app:

```
[gosd] formatting /dev/mmcblk0p2 as FAT32 labelled hello-data (59.2GiB) — one-time first-boot setup
[gosd] data partition created, filling the card
[gosd] data partition mounted read-write at /data
```

Boot 2, after a plain power cycle, HALTED:

```
[gosd] fatal: the data partition is corrupt: /dev/mmcblk0p2 holds a ext4
filesystem labelled "atfs-data" where a FAT32 filesystem labelled hello-data
should be; halting
```

`atfs-data` is a leftover from whatever was on that bench card before. Reading
the card on a Mac shows the format DID persist and is perfectly healthy:

```
2:  Windows_FAT_32  hello-data  63.6 GB  disk4s2
```

So a healthy, correctly-formatted device halted itself.

## Root cause

`diskfmt.FormatFAT32` delegates to go-diskfs `CreateFilesystem`, which writes
the FAT boot sector, FSInfo, their backups and the FAT tables — but **nothing
at offset 1024**, which sits in FAT32's reserved-sector area and is exactly
where an ext4 superblock lives. A previous ext4 volume's superblock therefore
survives a completely successful FAT32 format.

`diskfmt.Inspect` then probes **ext4 before FAT** (`isEXT4` = magic at 1024),
so it reports the dead ext4 volume — with its old label — in preference to the
live FAT32 one. gosd-init's `dataexpand` sees an established partition holding
"not the filesystem that format left", returns `ErrDataCorrupt` and halts.
The halt itself is correct and deliberate ("app data may be at stake"); it is
being fed a wrong answer.

## Scope: exactly one transition, proven

A reformat matrix over all six ordered pairs of {ext4, fat32, exfat} — format
A, format B, then Inspect — shows only **ext4 → FAT32** is affected:

| from → to | Inspect after reformat |
|---|---|
| **ext4 → fat32** | **ext4 "old-data"  ← WRONG** |
| ext4 → exfat | exFAT "new-data" (exFAT is probed before ext4) |
| exfat → fat32 | FAT32 "new-data" (FAT32 overwrites the OEM name at offset 3) |
| fat32 → ext4 | ext4 "new-data" |
| fat32 → exfat | exFAT "new-data" |
| exfat → ext4 | ext4 "new-data" |

## Reproduction (no hardware, ~1s)

Drop into `internal/diskfmt` as a `_test.go` file:

```go
func TestFAT32OverEXT4StillInspectsAsEXT4(t *testing.T) {
	dev := filepath.Join(t.TempDir(), "device.img")
	f, _ := os.Create(dev)
	_ = f.Truncate(768 << 20)
	f.Close()

	if err := FormatEXT4(dev, "atfs-data"); err != nil {
		t.Fatal(err)
	}
	if err := FormatFAT32(dev, "hello-data"); err != nil {
		t.Fatal(err)
	}
	got, err := Inspect(dev)
	if err != nil || got.FS != FAT32 || got.Label != "hello-data" {
		t.Fatalf("got FS=%v label=%q err=%v, want FAT32 \"hello-data\"", got.FS, got.Label, err)
	}
}
```

## Who this bites

- **Anyone who switches `--data-filesystem` from ext4 back to fat32** and
  reflashes. CLAUDE.md's locked decision for that ABI change promises the data
  partition is "cleanly reformatted, never halted" — this bug delivers exactly
  the halt the decision rules out.
- **Any recycled SD card** that previously held an ext4 filesystem at the data
  partition's offset (a Pi OS card, an Armbian card, an earlier gosd ext4
  image), flashed with a default FAT32-data image. First boot looks perfect;
  the device halts on the *second* boot, which makes it read as a random
  hardware fault rather than a card-history problem.
- **`emmc` and `disk`** share `diskfmt`, so the same stale superblock can make
  an eMMC/NVMe volume formatted FAT32 (e.g. `examples/usbwebsite`, which
  deliberately chooses FAT32 so hosts can read it) refuse adoption on the next
  boot.

The delayed onset is the nastiest part: the format, the mount and the app all
succeed, so nothing is visibly wrong until a reboot that may be days later.

## Fix direction (not implemented — needs a crash-ordering argument)

Erase competing signatures as part of establishing a volume, rather than
teaching `Inspect` to prefer one guess over another: before writing FAT32,
zero at least the ext4 superblock window (offset 1024..2048), and ideally
every signature offset the probes look at, then sync before the filesystem
write — the same write → sync → marker → sync discipline the rest of the
codebase uses. Making `Inspect` prefer a self-consistent FAT32 boot sector
over a bare ext4 magic would paper over it while leaving genuine ext4 debris
indistinguishable from a live volume.

## Todos

- [x] Land the reproduction above as a regression test
- [x] Erase competing filesystem signatures when establishing a volume, with
      an explicit crash-ordering argument
- [x] Check the same stale-signature question for the boot partition and for
      `emmc`/`disk` adoption paths


## Summary of Changes

`diskfmt.FormatFAT32` now zeroes the first 1 MiB of the device (a new
`eraseLeadingRegion`) before go-diskfs writes its boot sector, so a
successful format leaves exactly one identifiable filesystem behind.

The erase span is deliberately `blankProbeBytes` — the very span `Inspect`
reads before running every probe — which makes it both necessary and
sufficient: `isExFAT` (offset 3), `isEXT4` (offset 1024) and go-diskfs's own
FAT detection all resolve inside that window, so nothing identifiable can
survive outside it.

**Crash ordering.** The erase is issued before the formatter's own writes,
and this package never fsyncs mid-format (durability is the caller's single
sync at the end of an establish sequence), so the argument is about order
and reachable states: lose power before the erase lands and the device reads
exactly as it did before `Format` was called; lose power after it lands but
before the new signatures do and every probed byte is zero, so `Inspect`
reports `Blank: true` — the documented safe "nothing to destroy" state,
never a foreign filesystem; lose power partway through the formatter's own
writes and it is an interrupted format exactly as before, no worse. No
interruption can leave a foreign filesystem's magic intact underneath an
otherwise-complete different one, which is the single state this closes.

`FormatExFAT` and `FormatEXT4` were checked and deliberately left alone —
each already overwrites the whole probed window (exFAT writes its Main and
Backup Boot Regions across [0, 12288); ext4 streams a >=512 MiB golden from
offset 0). Both now carry a comment saying so, so the guarantee is
documented rather than accidental.

### The third todo, answered

- **`emmc` and `disk`** inherit the fix with no changes of their own: both
  format through `internal/blockmount`, which calls these same
  `diskfmt.Format*` functions — there is no second formatting path.
  `dataexpand` likewise (`platform_linux.go` wires `diskfmt.FormatFAT32` /
  `FormatEXT4` straight in).
- **The boot partition** was never exposed: it is built into a freshly
  created (all-zero) image file by `internal/image`, and flashing rewrites
  that whole region on the card, so there is no prior filesystem underneath
  it to survive.

### Test

`TestReformatOverwritesEveryPriorSignature` drives all six ordered pairs of
{ext4, fat32, exfat}. Confirmed it reproduces the bug on the unfixed code —
`Inspect after ext4 -> FAT32 = {FS:ext4 Label:OLD-DATA ...}`, exactly the
hardware symptom — and passes with the fix, with the other five pairs
unaffected.
