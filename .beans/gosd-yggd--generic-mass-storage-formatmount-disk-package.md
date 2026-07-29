---
# gosd-yggd
title: Generic mass-storage format/mount (`disk` package)
status: in-progress
type: feature
priority: normal
created_at: 2026-07-29T15:29:09Z
updated_at: 2026-07-29T15:45:41Z
parent: gosd-jge2
---

A public `disk` package that does for any attached mass-storage disk (NVMe SSD,
USB drive, SD card, plug-in eMMC) what [[gosd-tdcc]]'s `emmc` package does for
the soldered-on eMMC: discover it, format it on first use, mount it on every
boot after.

## JP's request (locked, verbatim)

> "Build an equivalent of the `emmc` package for NVMe drives — call it `disk`,
> and genericise it for any mass storage disk (usb drives or nvme). Same API
> please; and if we need to extract stuff from within `emmc` that's actually
> suitable for generic mass storage stuff then do."

Motivation: NVMe on the ROCK 4SE is hardware-proven ([[gosd-sz6p]]: /dev/nvme0n1
enumerated, 840 MB/s, exFAT mounted) but only ever via a throwaway app calling
`unix.Mount` directly. No package has ever existed for it.

## Design decisions (2026-07-29)

### 1. Shared code lives in a new `internal/blockmount`; both public packages are thin

`emmc` is public API and must keep working *identically* — same signatures,
same error identities, same wording — so `emmc` does NOT delegate to `disk`
(that would leak `disk`'s sentinels into `emmc`'s error chains and force a
"restrict to MMC" selector into `disk`'s public surface). Instead the generic
half moved down into `internal/blockmount`, which both packages parameterise:

- `blockmount.Storage{Pkg, Noun, Deps}` — `Pkg` prefixes label-validation
  errors (`"emmc: …"` / `"disk: …"`), `Noun` names the thing in messages
  (`"the eMMC at /dev/mmcblk1"` / `"the disk at /dev/nvme0n1"`).
- `blockmount.Run` — the mount-only / format / refuse orchestration.
- `blockmount.ValidateLabel` — FAT's 11-char printable-ASCII rule.
- `blockmount.Device` / `Choose` / `InUse` — the sysfs candidate model and the
  deterministic "not the boot device, not in use" selection, with each package
  supplying its own suitability predicate, ordering and not-found sentinel.
- `blockmount/platform_linux.go` — `/proc/mounts` parsing, `unix.Mount(vfat)`,
  `unix.Unmount`, `/sys/block` enumeration. Previously duplicated-in-waiting.

`ErrRefusedFormat` is now one value (`blockmount.ErrRefusedFormat`) re-exported
by both packages, so `errors.Is(err, emmc.ErrRefusedFormat)` still holds for
existing callers and means the same thing in both. `ErrNoEMMC` and `ErrNoDisk`
stay distinct — they answer different questions.

`internal/emmcfmt` renamed to `internal/diskfmt`: it was already generic
(`Inspect(path)`, `FormatFAT32(path, label)`) and is internal, so the rename is
free and stops the name lying.

### 2. Discovery: allowlist of device classes, ordered by class, never the boot device

`/sys/block` lists plenty of things a format target must never be, so `disk`
uses an **allowlist**, not a denylist:

| Accepted | Why |
| --- | --- |
| `nvme*n*` | NVMe namespaces — the flagship case |
| `sd*` | SCSI/USB mass storage — USB sticks and enclosures |
| `vd*` | virtio disks — qemu-virt and VMs |
| `mmcblk*` | SD cards and eMMC (excluding `…boot0/1`, `…rpmb`) |

Everything else is excluded by construction, and specifically: `loop*` (files,
not media), `ram*`/`zram*`/`zd*` (volatile RAM-backed), `dm-*` (device-mapper
virtual nodes — formatting one corrupts whatever it maps), `md*` (MD RAID
members), `sr*`/`scd*` (optical, not writable this way), `nbd*` (network block
devices), `mtdblock*`/`ubiblock*` (raw-flash translation layers, not mass
storage), and `mmcblk*boot*`/`mmcblk*rpmb` (eMMC hardware partitions holding
boot code, not general storage).

Three further exclusions apply to every candidate:

- **In use** — the whole device or *any* of its partitions appears in
  `/proc/mounts`. This is what makes the boot device off-limits (same rule
  `emmc` has always used), so an app can never wipe the media it is running
  from.
- **Zero size** (`/sys/block/<n>/size == 0`) — an empty USB card-reader slot
  enumerates as `sdb` with no medium.
- **Read-only** (`/sys/block/<n>/ro == 1`) — a write-protected card; better to
  report "no disk" than to fail deep inside a format.

**Ordering** when several candidates survive: by class — `nvme` > `sd` > `vd` >
`mmcblk` — then lexicographically within a class. Rationale: prefer the
deliberately-fitted, high-capacity device; leave the onboard eMMC last, since
the `emmc` package addresses it directly. Fully deterministic and independent
of kernel enumeration order.

**Selector for the ambiguous case** (additive, optional — the zero-config call
is unchanged):

- `disk.Devices()` lists the candidates in the same order discovery would pick
  them.
- `disk.FormatAndMountDevice(device, label, mountpoint, destructive)` targets
  one explicitly. It still refuses a device that is in use, so naming the boot
  device by hand cannot wipe the running system.

Partition enumeration is now scheme-independent: a child of `/sys/block/<n>`
counts as a partition if it contains a `partition` file. `emmc`'s old `<n>p*`
prefix rule was correct for `mmcblk0p1`/`nvme0n1p1` but would have missed
`sda1`.

### 3. Filesystem: whole-device FAT32, exFAT *named* but not mounted

`disk` keeps `emmc`'s whole-device FAT with no partition table, because that is
exactly what makes `Result.BlockDevice` directly shareable via
`gadget.MassStorage`.

exFAT verdict: **recognise, do not parse — follow-up bean.** `go-diskfs` v1.9.3
has no exFAT support at all (`filesystem.Type` is FAT12/16/32, ext4, iso9660,
squashfs), so mounting-only an existing exFAT volume would need hand-written
superblock *and* root-directory parsing (the exFAT volume label lives in a
`0x83` directory entry in the root directory, not in the boot sector) — real
on-disk-format work, out of scope here.

What was in scope and is done: `diskfmt.Inspect` now identifies an exFAT
volume from its boot-sector magic and reports it as `Contents.OtherFS =
"exFAT"`, so the refusal message says *"the disk at /dev/nvme0n1 already holds
an exFAT volume"* instead of the useless *"non-FAT content"*. Behaviour is
unchanged — it is still refused without `destructive`, which is the safe and
correct answer.

Consequence documented in `docs/runtime.md`: FAT32's 4 GiB per-file ceiling is a
real limit on a big NVMe, and reformatting an exFAT SSD as FAT32 is destructive
and one-way. Follow-up bean for exFAT read/mount support: not yet filed —
see "Follow-ups" below.

### 4. Naming

`ErrNoDisk` ("no usable disk found"), doc comments written for the generic case,
and error strings that name the actual device node.

## Structure

- `internal/blockmount/` — shared orchestration, label validation, candidate
  model + selection, and the Linux platform primitives.
- `internal/diskfmt/` — was `internal/emmcfmt`; + exFAT identification.
- `disk/disk.go`, `disk/platform_linux.go`, `disk/platform_other.go` — the new
  public package.
- `emmc/` — same public API, now a thin parameterisation of `blockmount`.

## Todo

- [x] `internal/blockmount` with shared Run/ValidateLabel/Choose/InUse + Linux
      platform primitives
- [x] `internal/emmcfmt` → `internal/diskfmt`, plus exFAT identification
- [x] `emmc` refactored onto `blockmount`, public API and error identities
      unchanged
- [x] public `disk` package: `FormatAndMount`, `FormatAndMountDevice`,
      `Devices`, `Unmount`, `Result`, `ErrNoDisk`, `ErrRefusedFormat`
- [x] Behavioural tests incl. table-driven discovery (NVMe, USB, several
      candidates, boot device excluded, nothing suitable, excluded node
      classes); pass on macOS
- [x] `docs/runtime.md` section, `COMPATIBILITY.md` NVMe footnote,
      `CLAUDE.md` public-API-surface fix
- [ ] Bench validation (see below)

## Bench validation (not yet done — needs hardware)

- [ ] ROCK 4SE + M.2 NVMe: `disk.FormatAndMount` discovers `/dev/nvme0n1`
      (not the boot microSD), formats a blank drive, mounts it, and a re-run
      mounts without reformatting.
- [ ] ROCK 4SE + the exFAT-formatted KIOXIA from [[gosd-sz6p]]: refused with
      `ErrRefusedFormat` and the message names it as an exFAT volume;
      `destructive=true` reformats it FAT32 and mounts it.
- [ ] FAT32-on-a-big-drive sanity: confirm go-diskfs's whole-device FAT32
      format completes on a 512 GB device and the kernel mounts the result.
- [ ] `disk.Unmount` then `gadget.MassStorage` on the same `BlockDevice`: the
      host sees the drive's contents.
- [ ] Any board + a USB stick: discovered as `/dev/sda`, formatted, mounted.
- [ ] Two disks attached (NVMe + USB stick): zero-config call picks the NVMe;
      `Devices()` lists both; `FormatAndMountDevice` targets the stick.
- [ ] A board with nothing attached: `ErrNoDisk`, app degrades gracefully.

## Follow-ups (not in this PR)

- exFAT read support in `diskfmt` (identify + label match) so an existing exFAT
  disk can be mount-only, and eventually exFAT *formatting* so files >4 GiB are
  possible on big disks.
- An `examples/diskstorage` worked example — deliberately skipped here because
  it would be a near-copy of `examples/emmcstorage` with one import changed;
  `docs/runtime.md` points at `examples/emmcstorage` for the shape.

## Quality gates

`go test ./...`, `go vet ./...`, `gofmt -l .` (empty), both
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`.

## Summary of Changes

Code complete; stays in-progress pending the bench validation above (no NVMe
board on the bench this session).

New public `disk` package: `FormatAndMount(label, mountpoint, destructive)` —
the same signature, `Result` shape and semantics as `emmc.FormatAndMount` —
discovers whatever mass storage is attached and is not the media the board
booted from, formats it whole-device FAT32 when blank (or when the caller opts
into overwriting), and mounts it read-write. Additive extras for the
two-disks-attached case: `Devices()` lists the qualifying nodes best-first and
`FormatAndMountDevice(device, …)` targets one by name, still refusing anything
in use. Plus `Unmount`, `ErrNoDisk` and `ErrRefusedFormat`.

Discovery is an allowlist (`nvme*`, `sd*`, `vd*`, `mmcblk*`) ordered by class,
so `loop`/`ram`/`zram`/`zd`/`dm-`/`md`/`sr`/`nbd`/`mtdblock`/`ubiblock` nodes
and eMMC `boot0`/`boot1`/`rpmb` hardware partitions can never be format
targets, and neither can a device with no medium, one that is write-protected,
or one with anything mounted from it — which is what keeps the boot media
off-limits, and holds because gosd-init leaves GOSD-BOOT mounted for the life
of the boot (see [[gosd-pcwl]] for the one transient exception, which it
unmounts).

The generic half of `emmc` moved down into `internal/blockmount` — the
mount-only/format/refuse orchestration, FAT label validation, the candidate
model and deterministic selection, and the Linux `/proc/mounts` + `unix.Mount`
+ `/sys/block` primitives — with `emmc` and `disk` now thin parameterisations
differing only in noun, sentinel and suitability rule. `emmc`'s public API,
wording and error identities are unchanged; `ErrRefusedFormat` is one shared
value both packages re-export. `internal/emmcfmt` became `internal/diskfmt`
and learned to identify exFAT from its boot-sector magic, so refusing a drive
that arrived exFAT-formatted now says so instead of "non-FAT content".

Partition enumeration is now scheme-independent (a `/sys/block/<n>` child with
a `partition` attribute), which the old `<n>p*` prefix rule needed to become —
it would have missed `sda1`.

Docs: new "Attached disk storage" section in `docs/runtime.md`, a `disk` row +
`[^disk]` footnote in `COMPATIBILITY.md` (🚧 until bench-verified), the
`[^rock4se-nvme]` footnote redirected from "mount it yourself with
`unix.Mount`" to the package, and `CLAUDE.md`'s public-API-surface bullet
corrected — it named a `device/` package that has never existed and omitted
`emmc/`.
