---
# gosd-1ici
title: 'exFAT in the disk package: mount an existing volume, format a new one'
status: in-progress
type: feature
created_at: 2026-07-29T16:13:42Z
updated_at: 2026-07-29T16:13:42Z
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

`disk.Supports(fs)` reads `/proc/filesystems` (authoritative on GoSD, which
builds module-less kernels) and the shared orchestration checks it *before*
formatting — so asking for exFAT on a board whose kernel lacks
`CONFIG_EXFAT_FS` fails with "this board's kernel has no exFAT support" and an
untouched disk, rather than a bare `mount: ENODEV` after a successful format.
The same check catches an existing exFAT volume on a board that cannot mount it.

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

- [ ] `internal/diskfmt`: exFAT boot-sector parse + root-directory walk for the
      volume label; `Contents.FS` replaces `IsFAT`
- [ ] `internal/blockmount`: mount type follows the filesystem found/created;
      kernel-support precheck
- [ ] `internal/diskfmt`: pure-Go exFAT formatter (boot regions + checksum,
      FAT, bitmap, up-case table, root directory)
- [ ] `disk`: `Filesystem`, `Options`, `FormatAndMountWith`, `Supports`
- [ ] Kernel fragments: assert exFAT on the Pi boards; decide for the Rockchip
      boards
- [ ] `docs/runtime.md` + `COMPATIBILITY.md`
- [ ] Behavioural tests, macOS-passing

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

## Quality gates

`go test ./...`, `go vet ./...`, `gofmt -l .` (empty), both
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`.
