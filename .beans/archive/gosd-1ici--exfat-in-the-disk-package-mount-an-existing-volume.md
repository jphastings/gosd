---
# gosd-1ici
title: 'exFAT in the disk package: mount an existing volume, format a new one'
status: completed
type: feature
priority: normal
created_at: 2026-07-29T16:13:42Z
updated_at: 2026-08-08T19:50:12Z
parent: gosd-jge2
---

Follow-up to [[gosd-yggd]], which shipped the `disk` package able to *name*
exFAT but not read, mount or write it. This bean adds both halves: mounting an
exFAT volume that is already there, and formatting one on request.

## JP's request (locked, verbatim)

> "add exFAT as an option to the new `disk` package (which currently
> formats/mounts whole-device FAT32 only, and merely *recognises* exFAT in order
> to refuse it)."

With the scope split locked as:

> **Phase 1 — read/mount exFAT (higher value, smaller):** teach `diskfmt.Inspect`
> to read an exFAT volume's LABEL (the `0x83` volume-label directory entry in the
> root directory — requires parsing the boot sector's FAT/cluster geometry and
> walking the root-directory cluster chain), so `disk`'s existing decision logic
> gains the mount-only path for an exFAT volume whose label matches: mount it,
> never reformat it. This is the realistic case — SSDs and USB drives ship exFAT,
> and JP's bench NVMe (a KIOXIA "betamin") already holds an exFAT filesystem
> they'd rather mount than wipe. Mounting is a `unix.Mount` filesystem-type
> argument, so it must pass "exfat" rather than "vfat" for such a volume.
>
> **Phase 2 — format as exFAT (the "option" JP asked for):** a minimal, correct
> pure-Go exFAT formatter in `internal/diskfmt`: main+backup boot regions with
> the VBR checksum sector, FAT, allocation bitmap, up-case table, and a root
> directory carrying the volume label. Whole-device (no partition table),
> matching the FAT32 path's shape so `Result.BlockDevice` stays directly
> shareable via `gadget.MassStorage`. A filesystem that Linux's `exfat` driver
> and macOS both mount is the bar.
>
> `disk.FormatAndMount(label, mountpoint, destructive)` stays exactly as it is
> (FAT32 remains the default). `emmc`'s public API must remain untouched and
> FAT-only.

## Design decisions

### 1. No third-party exFAT dependency — we write the formatter

Survey (2026-07-29): `go-diskfs` v1.9.3 (our pin) has no exFAT at all.
`github.com/dsoprea/go-exfat` is read-only and dormant.
`github.com/go-filesystems/exfat` does have a pure-Go `Format()`, but as of this
survey it has no tagged release, no importers, and its first publication is
weeks old — not something to hand a user's SSD to. Its `Format` is also
file-shaped (path + size, size must be a multiple of 4096) rather than
device-shaped, so it would not have removed much of the work anyway.

We therefore write the exFAT formatter ourselves against the Microsoft exFAT
specification. It is a bounded, fully-testable amount of on-disk-format work,
and the alternative was an unvetted dependency in the one code path that can
destroy a user's data.

### 2. `diskfmt.Contents` gains a filesystem *identity*, replacing the `IsFAT` bool

`Contents.IsFAT bool` becomes `Contents.FS diskfmt.FS` (`""`, `"fat32"`,
`"exfat"`). The decision logic in `blockmount.Run` was already "does the label
match?"; the only new fact it needs is *which* filesystem matched, because that
is what `unix.Mount` has to be told. `OtherFS` survives for a filesystem we can
name but not handle — it is now fed by go-diskfs's own detection (ext4,
iso9660, squashfs) instead of exFAT, so the refusal message stays specific.

`Inspect` now checks the exFAT boot-sector magic **before** asking go-diskfs for
a FAT, because a real exFAT boot sector carries the `0xAA55` signature at offset
510 and a FAT probe is not guaranteed to reject it.

### 3. Mount-only wins over the caller's chosen filesystem

`blockmount.Run` mounts what it found when the label matches — whatever
filesystem that is — and only uses the caller's chosen filesystem when it is
actually going to format. So the *unchanged* `disk.FormatAndMount("BETAMIN", …)`
call now mounts an existing exFAT volume labelled `BETAMIN` instead of refusing
it. That is the realistic bench case and it is strictly non-destructive: the
alternative (refuse) pushes people towards `destructive=true`, which is worse.

### 4. API: one options-taking entry point, not a function matrix

`disk` already had two entry points (`FormatAndMount`, `FormatAndMountDevice`).
A `…As` variant per axis would have made four, and a fifth on the next axis.
Instead there is one general call the other two delegate to:

```go
type Filesystem string
const (FAT32 Filesystem = "fat32"; ExFAT Filesystem = "exfat")

type Options struct {
    Filesystem  Filesystem // zero value formats FAT32
    Device      string     // zero value discovers one
    Destructive bool
}

func FormatAndMountWith(label, mountpoint string, opts Options) <-chan Result
```

`Options`' zero value is exactly today's `FormatAndMount(label, mountpoint,
false)`, so the common path is unchanged in both signature and behaviour, and
`FormatAndMountDevice` becomes a one-line wrapper. `Filesystem` is a distinct
public type rather than an alias of the internal `diskfmt.FS`, so `internal/`
stays internal.

`emmc` gains nothing: it passes FAT32 and its public API is byte-for-byte
unchanged.

### 5. A kernel that cannot mount it is caught before anything is written

The shared orchestration reads `/proc/filesystems` (authoritative on GoSD,
which builds module-less kernels) *before* formatting, so asking for exFAT on a
board whose kernel lacks `CONFIG_EXFAT_FS` fails with "this board's kernel has
no exFAT support" and an untouched disk, rather than a bare `mount: ENODEV`
after a successful format. The same check catches an existing exFAT volume on a
board that cannot mount it.

Shipped as **`disk.ErrUnsupportedFS` only**, not the `disk.Supports(fs)` probe
originally sketched. Because the check runs before any write, the error *is*
the probe: `errors.Is(res.Err, disk.ErrUnsupportedFS)` tells a caller to fall
back to FAT32, knowing the disk is exactly as it was. A separate predicate
would have been a second way to ask one question.

### 6. The up-case table is generated, not embedded

Linux's `exfat_load_upcase_table` requires the table to cover the whole BMP
(`index >= 0xFFFF` after decompression), so the tempting 30-entry "ASCII-only"
table would be rejected. The alternatives were embedding Microsoft's recommended
5,836-byte compressed table (checksum `0xE619D30D`) or generating an equivalent
one.

**Generated**, from Go's own `unicode.ToUpper` over the BMP, then run-length
compressed with the spec's `0xFFFF <count>` scheme. Reasons: every reader
(Linux, macOS, Windows) loads the table *from the volume* and validates it
against the checksum written beside it, so nothing requires Microsoft's exact
byte sequence; it avoids transcribing 5.8 KB of table out of GPLv2 tooling into
an MIT repo; and it stays correct as Go's Unicode tables move. The ASCII
`a-z → A-Z` mappings the spec makes mandatory are asserted by a test.

### 7. Geometry

512-byte sectors (`BytesPerSectorShift = 9`, matching the FAT32 path's
`diskfs.SectorSize512`), one FAT, and Microsoft's cluster-size ladder: 4 KiB up
to 256 MiB, 32 KiB up to 32 GiB, 128 KiB beyond. `FatOffset` 128 sectors,
`ClusterHeapOffset` aligned up to a cluster. `FatLength` is sized from an upper
bound on the cluster count (computed as if the FAT took no space), which is
always sufficient because adding FAT sectors only reduces the cluster count.

## Todo

- [x] `internal/diskfmt`: exFAT boot-sector parse + root-directory walk for the
      volume label; `Contents.FS` replaces `IsFAT`
- [x] `internal/blockmount`: mount type follows the filesystem found/created;
      kernel-support precheck
- [x] `internal/diskfmt`: pure-Go exFAT formatter (boot regions + checksum,
      FAT, bitmap, up-case table, root directory)
- [x] `disk`: `Filesystem`, `Options`, `FormatAndMountWith`, `ErrUnsupportedFS`
- [x] Kernel fragments: assert exFAT on the Pi boards; decide for the Rockchip
      boards
- [x] `docs/runtime.md` + `COMPATIBILITY.md`
- [x] Behavioural tests, macOS-passing

## Host verification (done, 2026-07-29)

Apple's own exFAT implementation was used as an independent oracle against a
256 MiB image `FormatExFAT` produced, attached with
`hdiutil attach -imagekey diskimage-class=CRawDiskImage -nomount`:

- `/sbin/fsck_exfat -n` passes every stage — main boot region, system files,
  **upper case translation table**, file system hierarchy, active bitmap, and
  the recheck of both boot regions: *"The volume BETAMIN appears to be OK."*
  (Run it against the plain file instead of the attached device and it reports
  the boot region invalid — that is fsck's own `ioctl` failing to get the block
  count/size on a regular file, not a defect in the image.)
- `diskutil info` identifies it as `File System Personality: ExFAT`,
  `Volume Name: BETAMIN`.
- macOS's `exfat.kext` **mounts it read-write**, creates `.fseventsd`, and
  round-trips a file across unmount/remount.
- `fsck_exfat` still passes *after* macOS has written to it, so the geometry we
  hand a real driver is one it can extend correctly.
- `diskfmt.Inspect` still reports `{FS:exFAT Label:BETAMIN}` after Apple's
  driver rewrote the root directory — the reader is not just reading back its
  own writer's layout.

That covers "a filesystem macOS mounts". The Linux half is the bench list
below; `CONFIG_EXFAT_FS` on the bench boards is what it needs.

## Bench validation (not yet done — needs hardware)

- [ ] ROCK 4SE + the exFAT KIOXIA ("betamin"): `disk.FormatAndMount` with the
      drive's own label mounts it **without wiping it**, and its existing files
      are readable.
- [ ] `disk.FormatAndMountWith(…, disk.Options{Filesystem: disk.ExFAT,
      Destructive: true})` on a scratch drive: formats, mounts, round-trips a
      file, and a re-run mounts it without reformatting.
- [ ] Write a >4 GiB file to the exFAT-formatted drive — the FAT32 ceiling is
      gone.
- [ ] `disk.Unmount` then `gadget.MassStorage` on the same `BlockDevice`: a
      macOS host mounts the exFAT volume the board formatted, sees the label,
      and reads the file.
- [ ] `fsck.exfat` (exfatprogs) on the board-formatted volume reports no errors.
- [ ] A board whose kernel lacks `CONFIG_EXFAT_FS`: asking for exFAT fails with
      the "no exFAT support" error and the disk is untouched.

## Follow-ups (deliberately not in this PR)

- **exFAT on `qemu-virt`.** Its kernel has `# CONFIG_EXFAT_FS is not set`,
  so CI cannot mount what the formatter writes. Enabling it plus a boot test
  that formats a scratch virtio disk as exFAT and mounts it would be the
  strongest possible verification short of the bench — but it is a CI test,
  a fragment change and an artifacts release for an internal-only board, so
  it belongs in its own bean rather than riding this one.
- **4Kn devices.** The formatter fixes `BytesPerSectorShift = 9`, matching
  the FAT32 path's `diskfs.SectorSize512`. A native-4K-sector NVMe would need
  the shift derived from `BLKSSZGET`. No such device is in scope, and the
  FAT32 path has the same assumption.
- **go-diskfs sizes block devices with `unix.IoctlGetInt`**, which passes a
  pointer to Go's `int` — 4 bytes on `GOARCH=arm` — to `BLKGETSIZE64`, which
  writes 8. Noticed while reading its device-size path for the formatter.
  It affects the existing FAT32 path on pi-zero-w identically, so it is not
  a regression here, but it is worth a bean of its own.

## Quality gates

`go test ./...`, `go vet ./...`, `gofmt -l .` (empty), both
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`.

## Summary of Changes

Both phases landed. Stays in-progress pending the bench checklist above (no
NVMe board on the bench this session).

**Phase 1 — mount what is already there.** `internal/diskfmt` parses an exFAT
boot sector's geometry and walks the root directory's cluster chain for its
`0x83` volume-label entry, so `Contents` now reports *which* filesystem a disk
carries (`Contents.IsFAT` became `Contents.FS`) rather than just "FAT or not".
The shared orchestration mounts an existing volume as the filesystem it
actually is whenever its label matches the app's, so the unchanged
`disk.FormatAndMount("BETAMIN", …)` now mounts an exFAT drive labelled
`BETAMIN` instead of refusing it — which is the whole point, since that data
is why the drive was plugged in. `blockmount.MountVFAT` became
`blockmount.Mount(device, mountpoint, fs)`, passing `exfat` or `vfat` to
`mount(2)` and dropping the vfat-only `flush` option for exFAT, which would
otherwise be rejected. exFAT is probed before go-diskfs is asked to guess,
because a real exFAT boot sector carries the same `0xAA55` signature a FAT
probe looks for.

**Phase 2 — the formatter.** `FormatExFAT` writes main and backup boot regions
(with the VBR checksum sector), the FAT, the allocation bitmap, a
BMP-complete up-case table and a root directory carrying the volume label —
whole-device, no partition table, so `Result.BlockDevice` stays directly
shareable via `gadget.MassStorage` exactly as the FAT32 path is. Geometry
follows Microsoft's cluster ladder (4 KiB / 32 KiB / 128 KiB by volume size)
with 512-byte sectors and one FAT; `FatLength` is sized from an upper bound on
the cluster count, which is always sufficient and avoids the circular
dependency between the two fields.

**Verification (no hardware yet).** Tests assert the properties a driver
checks rather than mocking: the boot checksum recomputed over sectors 0-10
matches the checksum sector, with offsets 106/107/112 excluded as the spec
requires; the backup boot region is byte-identical to the main one; all eight
extended boot sectors carry `0xAA550000`; the allocation bitmap agrees exactly
with the clusters the FAT chains reach, in both directions; the up-case table's
recorded checksum matches the bytes on disk, and decompressing it the way
Linux's `exfat_load_upcase_table` does yields all 65,536 entries with correct
ASCII/Latin/Greek/Cyrillic mappings. The label round-trips through `Inspect`
across the whole cluster ladder (8 MiB, 1 GiB, 64 GiB). The reader is tested
against hand-built fixtures rather than the formatter's own output, so the two
halves are not marking each other's homework.

**API.** `disk.FormatAndMountWith(label, mountpoint, disk.Options{…})` with
`Options{Filesystem, Device, Destructive}`; `FormatAndMount` and
`FormatAndMountDevice` are now one-line wrappers over it and are unchanged in
signature and behaviour. `disk.Filesystem` is its own public type
(`disk.FAT32`, `disk.ExFAT`) rather than an alias of the internal one. New
sentinel `disk.ErrUnsupportedFS`. `emmc`'s public API is untouched and
FAT32-only.

**Kernels.** `CONFIG_EXFAT_FS` + `CONFIG_NLS_UTF8` asserted in the pi-zero-2w,
pi-zero-w and pi-3b fragments (previously defconfig luck; compiled kernels
unchanged, but they now appear in kernelspec's `RequiredY` so a trim cannot
cut them silently), and newly enabled for radxa-zero-3e and nanopi-zero2 —
both have USB host ports, and a per-board answer to "can this mount the drive
I plugged in?" is a footgun app authors would have to carry. `RequiredY` for
those two boards updated to match. No `artifacts.Version` bump: the two
Rockchip kernels change, so this reaches real builds at the next artifacts
release, which COMPATIBILITY.md's new "exFAT on attached disks" row states —
✅ for the three Pi boards and the ROCK 4SE today, 🚧 for the two Rockchip
boards until then.



---

Closed 2026-08-08 (end-of-session triage): deliverable shipped and on main; status was never flipped from in-progress. Reopen if a hardware sign-off is still outstanding.
