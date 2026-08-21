# ext4 golden images

A golden image is a small, pristine ext4 filesystem, zstd-compressed and
checked into this repository. It is the seed for GoSD's ext4 "format by
golden copy" route (epic gosd-lfu0): the formatter decompresses one,
raw-writes it to the target block device, stamps a fresh per-volume UUID and
label into the superblock (bean gosd-apmv), mounts it, and -- where the
volume is meant to be bigger than its seed -- grows it to the partition's
real size with the kernel's `EXT4_IOC_RESIZE_FS` ioctl (bean gosd-1c0x).
There is no `mkfs.ext4` or `resize2fs` on the device -- gosd-init stays
userland-free (see the epic's locked decisions).

There are two, and one image genuinely cannot do both jobs:

| Asset | Size | Journal | Seeds | Grows? |
|---|---|---|---|---|
| `golden.img.zst` | 512 MiB | 128 MiB | `/data` on an ext4 image, and every `emmc`/`disk` volume an app formats | Always, once, at first establishment |
| `config-golden.img.zst` | 32 MiB | 4 MiB | the config partition (bean gosd-onjv) | Only if `--config-size` exceeds it |

The split exists because **the journal can never be resized after format**
(see "Journal size" below), so a seed's journal has to be sized for the
filesystem it will eventually become -- which is what puts a 512 MiB floor
under the data golden and makes it unusable as the seed for a 32 MiB
partition. The two are otherwise built from the same recipe with the same
pins, and differ in exactly three parameters: total size, journal size, and
`resize_inode` vs `meta_bg`.

This document records *why* every mke2fs parameter was chosen. The exact
invocation lives in `../../../build/ext4-golden/Dockerfile`; regenerate with
`../../../build/ext4-golden/build.sh`.

## Parameters, and why

These are the **data** golden's parameters. The config golden shares every
one of them except the three called out in its own section below.

| Parameter | Value | Why |
|---|---|---|
| Block size | 4096 (4 KiB) | Matches the page size on every GoSD board's CPU; the standard modern default, and required by the group-size math below (32768 blocks = 128 MiB per block group). |
| `-O metadata_csum_seed` | on | Lets a later UUID change (gosd-apmv stamps a fresh one per volume) touch only the superblock, not every metadata checksum in the filesystem. See "metadata_csum_seed, verified" below. |
| `-O 64bit` | on | Group descriptors become 64 bytes (vs. 32) and the block-count fields grow to 64 bits, which is what makes a >16 TiB filesystem representable at all (`2^32` blocks × 4 KiB = exactly 16 TiB is the ceiling *without* it). |
| `-O meta_bg`, **not** `resize_inode` | on / off | The mechanism that makes online growth to ≥16 TiB possible in one step. See "Why meta_bg, not resize_inode" below -- this is the parameter the bean asked to research most carefully, and the answer isn't the obvious one. |
| `-E lazy_itable_init=1`, `-E lazy_journal_init=1` | on | Ship without fully zeroing the inode table or journal; the kernel finishes in the background after mount. See "Crash safety of the lazy flags" below. |
| `-J size=128` (128 MiB journal) | fixed | The journal does **not** grow when the filesystem is resized (see below) -- it has to be sized up front for the *grown* filesystem's lifetime, not the golden's 512 MiB. |
| `-U` (fixed placeholder UUID) | `4c1a41c8-20b8-4c50-8399-7fae324e8398` | Arbitrary but fixed, for byte-reproducible builds. gosd-apmv overwrites it with a fresh random UUID per volume at format time. |
| `-L` (label) | `""` (empty) | Same reasoning -- gosd-apmv stamps the real label at the same site, whatever the image was built with (per-app since gosd-lo7k; ext4's limit is 16 bytes, far more than a label needs). |
| `-E hash_seed` | `da89e13f-1cf4-4015-a4e0-0e9abbd2aabd` | Directory-hash seed; fixed rather than random, again for reproducible builds (mke2fs's own `-E hash_seed=` doc: "Intended for use with reproducible builds"). |
| `E2FSPROGS_FAKE_TIME=1735689600` | fixed | e2fsprogs-specific env var that fakes "now" for every on-disk timestamp (`s_mkfs_time`, `s_lastcheck`, inode ctime/mtime). Without it, every rebuild's raw image differs only in these bytes but still fails a byte-for-byte diff. |
| Golden virtual size | 512 MiB | See "Golden image size" below. |
| Inode size / ratio | 256 bytes / default (16384 B per inode) | Standard ext4 defaults. Not fixed at grow time -- see "Inode count on grow" below. |

### metadata_csum_seed, verified

Kernel.org's ext4 on-disk-format docs (`filesystems/ext4/super.html`) define
the superblock's checksum seed field precisely:

> `s_checksum_seed`: "Checksum seed used for metadata_csum calculations.
> This value is crc32c(~0, $orig_fs_uuid)."

and the feature itself:

> "Metadata checksum seed is stored in the superblock. This feature enables
> the administrator to change the UUID of a metadata_csum filesystem while
> the filesystem is mounted; without it, the checksum definition requires
> all metadata blocks to be rewritten."

In other words: every metadata checksum in the filesystem (group
descriptors, inode table entries, extents, directory blocks, bitmaps) is
computed from the *stored* seed, not the live UUID. This was also verified
empirically while building this recipe: on a populated test image,
`tune2fs -U <new-uuid>` left `dumpe2fs -h`'s "Checksum seed" field
byte-identical, and a subsequent `e2fsck -fn` reported the filesystem clean
-- proving the checksums computed from that seed (i.e. almost everything)
never needed touching. Only the superblock itself (the primary copy, plus
its handful of `sparse_super` backup copies -- a small, bounded count, not
one per block group) needs its own UUID field and self-checksum updated.

### Why meta_bg, not resize_inode

The bean asked specifically to verify that reserved-GDT-block sizing
(`resize_inode` + `-E resize=`) can support online growth to ≥16 TiB in one
step. **It can't, at 4 KiB blocks.** This was the single most important
finding of this bean, discovered empirically before being confirmed against
the docs:

`resize_inode`'s reserved GDT blocks are referenced through the resize
inode's *single indirect block* -- a hard architectural cap of
`blocksize / 4` reserved GDT blocks, i.e. **1024 blocks at 4 KiB**, no matter
how large a target `-E resize=` requests. Verified directly: formatting a
512 MiB image with `-E resize=<blocks>` targeting 8 TiB, 16 TiB, 32 TiB and
64 TiB all converged on exactly 1024 reserved GDT blocks (1023 at an 8 TiB
target, the cap from ~8 TiB upward). 1024 blocks × 64-byte descriptors ×
128 MiB/group works out to **~8 TiB** -- below this bean's 16 TiB
requirement.

`meta_bg` removes that ceiling by storing group descriptors as ordinary
data blocks distributed across each *meta block group*, instead of
reserving them contiguously up front. Kernel.org's
`filesystems/ext4/blockgroup.html`:

> "Without the option META_BG, for safety concerns, all block group
> descriptors copies are kept in the first block group... This increases
> the 2^21 maximum block groups limit to the hard limit 2^32, allowing
> support for a 512PiB filesystem... Existing filesystems can be resized
> on-line."

`resize_inode` and `meta_bg` are mutually exclusive at format time --
mke2fs refuses to enable both ("The resize_inode and meta_bg features are
not compatible. They can not be both enabled simultaneously."), so this
golden image ships with `^resize_inode,meta_bg` explicitly.

**Growth ceiling actually proven:** `build/ext4-golden/build.sh` runs a
verification step in a privileged container (see below) that loop-mounts
the golden image, writes a file, and online-resizes a truncated sparse copy
while mounted -- the exact `EXT4_IOC_RESIZE_FS` kernel path GoSD's `disk`
package uses. The target size it attempts is 16 TiB, falling back to 8
TiB / 4 TiB / 1 TiB / 256 GiB if the build host's *own* filesystem can't
even represent a file that large (`manifest.json`'s
`verification.verifiedGrowthCeilingBytes` records whichever ceiling the
last run actually proved). On the sandbox this recipe was developed in,
the build host's own root filesystem (a non-64bit ext4, colima's default
VM image) tops out at just under 16 TiB, so the proven run landed at 8 TiB
-- a build-host limitation, not a property of this golden image or its
`meta_bg` parameters. In every run: the filesystem mounted, a file written
before the grow survived it, and `e2fsck -f` afterward was clean. Combined
with the `2^32`-group / 512 PiB ceiling `meta_bg` is documented to support,
there is no realistic ceiling below 16 TiB -- re-proving the literal 16 TiB
(or larger) figure just needs a build host whose own filesystem isn't
itself capped, which qemu-virt's CI environment (bean gosd-ucgr) is a
better fit for than a maintainer's local Docker Desktop/colima VM.

### Crash safety of the lazy flags

`lazy_itable_init=1` ships the golden with inode tables marked
`BG_INODE_UNINIT` rather than zeroed. The kernel's `ext4lazyinit` thread
zeroes each block group's table in the background after mount, clearing the
flag only once that group is done. A crash mid-init simply leaves the flag
set on whichever groups haven't been zeroed yet -- the flag (itself covered
by the group descriptor's checksum) is the sole source of truth for "is this
inode table's content meaningful", so the kernel never treats a
not-yet-zeroed region as containing valid inodes, and resumes zeroing
un-flagged groups on the next mount. `lazy_journal_init=1` is the same
class of feature for the 128 MiB journal: journal recovery only trusts
blocks whose commit record matches the current sequence number, so
whatever non-zero bytes were left by skipping the explicit zero-fill are
simply never replayed as a transaction.

### Journal size: fixed, does not scale with growth

Neither `resize2fs` nor the kernel's online-resize ioctl touch the journal
inode -- it stays whatever size it was formatted with, forever. e2fsprogs'
own default-sizing table (`lib/ext2fs/mkjournal.c`,
`ext2fs_default_journal_size`) tops out at a 1 GiB journal for filesystems
of 128 GiB or larger:

```
< 32768 blocks (128 MB):   4 MB      < 512K blocks (2 GB):    32 MB
< 256K blocks (1 GB):     16 MB      < 4096K blocks (16 GB):  64 MB
< 8192K blocks (32 GB):  128 MB      < 16384K blocks (64 GB): 256 MB
< 32768K blocks (128 GB): 512 MB     >= 32768K blocks:          1 GB
```

If the golden's own small size drove the default (formatting at 512 MiB
picks a 16 MB journal), a multi-TiB grown volume would be stuck with a
journal sized for a filesystem 1/1000th its size. This recipe instead picks
**128 MiB** explicitly: e2fsprogs' own default for the 32-64 GiB bucket,
comfortably larger than an embedded appliance's config/data-file workload
needs (GoSD apps aren't running a high-throughput database -- the journal
buys metadata crash-consistency and mount-time replay, not data durability;
that's still `docs/runtime.md`'s fsync pattern, unchanged), without
inflating the golden's checked-in virtual size to match e2fsprogs' own
1 GiB ceiling for truly huge filesystems.

### Golden image size: 512 MiB

Balances the journal floor (128 MiB, above) against the checked-in asset's
compressed size and the meta_bg metadata's own room to grow. Since a fresh
`meta_bg` format doesn't pre-reserve GDT space the way `resize_inode` would,
a small golden costs almost nothing here -- the 512 MiB choice is really
just "journal (128 MiB) plus comfortable headroom for the superblock, root
directory, `lost+found`, and backup copies", landing inside the
256 MiB-1 GiB range the bean suggested. Compressed size is dominated by how
much of the image is zero bytes (the whole journal, most of the inode
table thanks to `lazy_itable_init`) -- zstd crushes it to **~17 KB**,
nowhere near the 1 MiB budget, so there was no pressure to shrink further.

### Inode count on grow

`s_inodes_per_group` is a single filesystem-wide superblock field (kernel.org
`filesystems/ext4/super.html`), not stored per-group -- so it applies
uniformly to every block group, *including the ones a grow operation
creates*. Unlike the journal, inode capacity is not fixed at format time:
growing this golden image from 512 MiB to 8 TiB in this recipe's
verification run took the inode count from 32,768 to 536,870,912,
proportional to the new capacity at the same (default) inode ratio. No
separate inode-count parameter was needed.

### Default mount behavior

`errors=` was left at mke2fs's default (`Errors behavior: Continue` in
`dumpe2fs -h`'s output for this image) rather than pinned to
`errors=remount-ro`. GoSD boards mount their internal drives without any
mount-option override today (`internal/blockmount`), so changing this would
need a corresponding blockmount change; out of scope for this bean, which
only produces the golden image. Worth revisiting alongside gosd-1c0x if a
board ever needs stricter on-corruption behavior than "keep going and let
the journal/fsck sort it out on next mount".

## The config golden: 32 MiB, 4 MiB journal, resize_inode

`config-golden.img.zst` seeds the config partition (bean gosd-onjv) -- a
partition every image carries, holding kilobytes of settings, that exists so
that an app sharing or serving its data volume cannot thereby publish the
device's own configuration. It differs from the data golden in three
parameters and nothing else.

| Parameter | Data | Config | Why the config golden differs |
|---|---|---|---|
| Golden size | 512 MiB | 32 MiB | The partition is 32 MiB by default (`gosd build --config-size`). The seed matches it, so the common case involves no resize at all -- see "Why not smaller" below. |
| `-J size=` | 128 MiB | 4 MiB | ext4's own minimum journal, 1024 blocks at the 4096-byte block size both goldens use. The data golden's 128 MiB is sized for a grown multi-terabyte filesystem; a partition that stays 32 MiB needs nothing of the sort. |
| Resize mechanism | `-O ^resize_inode,meta_bg` | `-O resize_inode` (mke2fs's own default) | `meta_bg` exists solely to escape `resize_inode`'s ~8 TiB ceiling, and a fixed-size partition cannot approach that ceiling. Keeping the feature set plain and standard means any host's `e2fsck` -- and any recovery tool somebody reaches for -- meets the filesystem shape it knows best. |

Everything else is identical and identically argued above: 4096-byte blocks,
`metadata_csum_seed` (so gosd-apmv's per-volume UUID stamp stays a
superblock-only edit), `64bit`, the lazy init flags and their crash-safety
argument, a fixed placeholder UUID and hash seed, an empty label, 256-byte
inodes and the default inode ratio.

### Why not smaller than 32 MiB

Because 4 MiB of it is journal, and that is the floor ext4 imposes rather
than one this recipe chose. A freshly formatted config golden reports
(`dumpe2fs`) 1543 overhead clusters -- roughly 6 MiB of journal, inode table,
bitmaps, `lost+found` and superblock -- leaving about 26 MiB free for
settings that are measured in kilobytes. Going below 32 MiB would buy back a
few megabytes of card space while making the journal a larger and larger
proportion of the volume; 32 MiB is where that trade stops being worth
making, and it aligns comfortably.

### Growth, when `--config-size` exceeds the seed

`--config-size` is a build-time flag, so a partition larger than the seed is
possible and is grown exactly once at establishment, through the same
`EXT4_IOC_RESIZE_FS` path the data golden uses (`internal/blockmount`). At
the default size no resize happens at all, which is both the simpler and the
overwhelmingly commoner path.

`resize_inode` bounds how far that growth can go, and here the bound is real
rather than theoretical: mke2fs reserved 3 GDT blocks in this golden (its
default target is 1024x the format size), which with 64-byte group
descriptors and 128 MiB block groups tops out around **32 GiB**. That is four
orders of magnitude past any plausible `--config-size` and is recorded here
so nobody has to rediscover it; it is emphatically not a reason to reach for
`meta_bg`, whose own ceiling argument (see above) is about the data golden's
job, not this one.

`build.sh`'s verification step proves the path rather than the ceiling: it
grows the config golden online from 32 MiB to 1 GiB while mounted, confirms a
file written beforehand survived, and requires a clean `e2fsck -f`
afterwards. `config-manifest.json`'s `verification.verifiedGrowthCeilingBytes`
records that 1 GiB target -- a size deliberately chosen as an implausibly
large config partition, **not** a discovered limit.

## Regenerating

```sh
../../../build/ext4-golden/build.sh          # both goldens
../../../build/ext4-golden/build.sh config   # just one
```

Requires Docker (or Podman via a docker-compatible context, e.g. colima)
and `jq`. Per golden, the script:

1. Builds e2fsprogs from source inside Docker, at the pinned tag + commit in
   `build.sh` (never the distro-packaged version -- that would drift with
   every Debian point release).
2. Runs the exact `mke2fs` invocation from `Dockerfile` to produce the raw
   golden image.
3. Runs `verify.sh` in a **privileged** container: loop-mounts the image,
   writes a file, grows a truncated sparse copy to as large a target as the
   build host allows (16 TiB down to 256 GiB) while mounted via
   `resize2fs`, confirms the file survived, and requires a clean `e2fsck -f`
   afterward. If the container can't get a working loop device at all (some
   CI/Docker setups block this even with `--privileged`), the script prints
   why and skips gracefully -- see the growth-ceiling discussion above; that
   case leaves runtime verification to the qemu-virt smoke test, bean
   gosd-ucgr.
4. Compresses with zstd (`github.com/klauspost/compress/zstd` decodes it on
   the Go side -- see `golden_test.go` -- standard zstd streams are fully
   interoperable regardless of which implementation wrote them).
5. Writes the compressed asset and its manifest (provenance: e2fsprogs
   version/commit, the exact mke2fs parameters, sha256 of both the raw and
   compressed image, and whichever growth target the verification step
   actually proved) into this directory.

### On determinism

The raw images *are* byte-for-byte reproducible: two independent runs of the
recipe (confirmed with `cmp`/`sha256sum` while developing each golden)
produce an identical `golden.img`, because every source of non-determinism
mke2fs has
is pinned -- fixed UUID, fixed hash seed, `E2FSPROGS_FAKE_TIME` for every
on-disk timestamp, and a pinned e2fsprogs build (no distro-package drift).
This is not a universal e2fsprogs guarantee (see
[tytso/e2fsprogs#91](https://github.com/tytso/e2fsprogs/issues/91), "mke2fs
cannot (easily) be made deterministic") -- it holds *for this specific,
fully-pinned invocation*, and was verified directly rather than assumed.
The compressed `.zst` bytes are not asserted byte-identical across regens
(zstd's own version/settings could in principle change that without
affecting the decompressed content); `manifest.json` records the sha256 of
both the raw and compressed artifact from the run that produced what's
checked in, which is what actually matters.

## Provenance

See `manifest.json` (data) and `config-manifest.json` (config) in this
directory: e2fsprogs repo/tag/commit, the full mke2fs parameter set, sha256
of the raw and compressed image, whether the in-container verification ran,
the growth target it proved, and the generation timestamp.

## Contract these assets provide

`golden_test.go` in this directory is the pure-Go, no-mount, no-Docker
behavioral pin on both checked-in assets: it decompresses each and asserts
the primary superblock's magic, exact feature-flag sets, block size, block
count, journal size, empty label, fixed UUID, and presence of the checksum
seed -- then cross-checks each asset against its manifest's recorded sizes
and digests, and against the `Golden` constant in `golden.go`, so those three
cannot drift apart silently. Bean gosd-apmv's superblock reader/writer (the
actual `Inspect` / `Format` implementation, `go:embed`-ing these assets)
should keep this test green -- if it doesn't, an asset's contract changed and
gosd-apmv needs to know.
