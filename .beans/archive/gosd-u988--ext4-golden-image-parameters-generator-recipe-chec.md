---
# gosd-u988
title: 'ext4 golden image: parameters, generator recipe, checked-in asset'
status: completed
type: task
priority: normal
created_at: 2026-08-07T09:57:52Z
updated_at: 2026-08-07T10:43:28Z
parent: gosd-lfu0
---

Design + produce the pristine ext4 golden image for epic gosd-lfu0's format-by-copy route, and the maintainer tooling that regenerates it.

## Todos

- [x] Pin the mke2fs parameter set, each with a recorded WHY: 4KiB blocks; -O metadata_csum_seed (so a per-volume UUID change touches only the superblock, not every checksum — verify this against the ext4 disk layout doc); 64bit + resize/reserved-GDT sizing so EXT4_IOC_RESIZE_FS can grow the golden size to >=16TiB in one online step (verify the reserved-GDT math actually covers it; if online grow to 16TiB needs meta_bg or a bigger golden, record the real ceiling and choose); lazy_itable_init semantics on first mount (kernel thread finishes zeroing — confirm crash-safe mid-init); journal size at golden size vs after grow (journal does NOT scale on resize — pick a size that is sane for multi-TiB and record it); default mount behavior expectations (errors=remount-ro?)
- [x] Golden image byte size choice (balance: checked-in compressed size, journal size floor, resize ceiling)
- [x] Maintainer regen recipe under build/ (Docker, pinned e2fsprogs version, deterministic: -U fixed, fixed timestamps via mke2fs env or post-strip) + manifest recording provenance; NOT part of any go build/test
- [x] Check in the compressed golden image + a pure-Go behavioral test that decompresses it and asserts superblock magic, feature flags, block count, label empty, csum_seed present (runs on macOS — no mount, just superblock parsing)
- [x] Record the exact regeneration + verification procedure in a README next to the asset
- [x] Quality gates + PR (https://github.com/jphastings/gosd/pull/186)



## Summary of Changes

Produced the ext4 golden image (`internal/diskfmt/ext4golden/golden.img.zst`, 17509 bytes compressed) plus the maintainer regen recipe (`build/ext4-golden/`: Dockerfile + build.sh + verify.sh) and its pure-Go contract test.

**Parameters chosen (all with WHYs in `internal/diskfmt/ext4golden/README.md`):**
- 4 KiB blocks; `-O metadata_csum_seed` (verified against kernel.org docs AND empirically via `tune2fs -U` + `e2fsck -fn` on a populated image: checksum seed unchanged, fsck clean -- only the superblock + its sparse_super backups need touching on a UUID change).
- `-O 64bit,meta_bg` with `resize_inode` explicitly DISABLED (`^resize_inode`) -- this was the key finding. `resize_inode`'s reserved-GDT-blocks mechanism has a hard architectural ceiling of `blocksize/4` = 1024 reserved GDT blocks (its reserved blocks are addressed through the resize inode's single indirect block), which caps online growth at ~8TiB with 4KiB blocks -- BELOW the bean's 16TiB target, confirmed empirically by sweeping `-E resize=` targets from 1TiB to 64TiB (reserved GDT blocks plateaued at 1024 from ~8TiB up). `meta_bg` removes that ceiling (distributes descriptors instead of reserving them contiguously) and is documented to support online resize up to 2^32 groups (512PiB). `resize_inode` and `meta_bg` are mutually exclusive at mke2fs format time.
- `-E lazy_itable_init=1,lazy_journal_init=1` -- crash-safety argument recorded (BG_INODE_UNINIT flag is the sole source of truth; journal recovery only trusts sequence-matched commit blocks).
- `-J size=128` (128MiB journal, fixed -- does not scale with resize; matches e2fsprogs' own default bucket for 32-64GiB filesystems, sane for a multi-TiB embedded-appliance workload without inflating the golden's checked-in size to match e2fsprogs' own 1GiB ceiling for truly huge filesystems).
- Fixed placeholder UUID + empty label (gosd-apmv stamps real per-volume values); fixed hash_seed + `E2FSPROGS_FAKE_TIME` for byte-reproducible builds (verified: two independent regen runs produced byte-identical raw images, confirmed via `cmp`/`sha256sum`).
- Golden image virtual size: 512MiB (journal floor + headroom; meta_bg does not pre-reserve GDT space, so there was no size pressure from the resize ceiling itself).

**Proven growth ceiling:** the in-container verification (privileged loop-mount + `resize2fs` while mounted + `e2fsck -f`) grows a truncated sparse copy of the golden image and proves the online-resize path actually works through the real kernel ioctl. It targets 16TiB, falling back to 8TiB/4TiB/1TiB/256GiB if the build host's own filesystem cannot represent a file that large. On this development sandbox (colima's default VM image runs a non-64bit ext4 root fs, itself capped just under 16TiB), the proven run landed at **8TiB** (`8796093022208` bytes) -- a build-host limitation recorded honestly in `manifest.json` and the README, not a property of the golden image's own parameters. Combined with the documented meta_bg mechanism (2^32 groups / 512PiB ceiling) there is no realistic ceiling below 16TiB; re-proving the literal 16TiB figure just needs a build host whose own filesystem isn't itself capped (qemu-virt CI, bean gosd-ucgr, is a better fit than a maintainer's local Docker/colima VM). In every run: the fs mounted, a file written pre-grow survived the online grow, and `e2fsck -f` was clean afterward.

**Asset size:** 17509 bytes compressed (well under the 1MiB target -- dominated by zero bytes from the journal and lazy-initialized inode table, which zstd crushes to almost nothing).

Quality gates (go test/vet/gofmt/golangci-lint incl. GOOS=linux) all pass. PR not yet opened.
