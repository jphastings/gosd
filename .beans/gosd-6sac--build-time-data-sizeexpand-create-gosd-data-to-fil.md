---
# gosd-6sac
title: 'Build-time data-size=expand: create GOSD-DATA to fill the card on first boot'
status: in-progress
type: feature
priority: normal
created_at: 2026-07-30T20:10:06Z
updated_at: 2026-07-30T20:43:49Z
---

Ship-small, fill-on-first-boot data partitions: `gosd build --data-size=expand` produces an image with NO data partition (image stays 272MiB, exactly today's `--data-size=0` layout), plus a flag in the baked `/etc/gosd/config.json` telling gosd-init to create and format `GOSD-DATA` spanning the rest of the physical card on first boot.

## Proposed design (for JP to confirm/lock)

**Why "no partition" rather than "smallest possible, then grow":** growing FAT32 in place means rewriting both FATs, FSInfo and the boot sector's sector counts — effectively a reformat-with-data-preservation that neither go-diskfs nor `internal/diskfmt` has. Creating the filesystem fresh at first boot reuses the existing tested formatter (`internal/diskfmt.FormatFAT32`) unchanged, and makes idempotency trivial. There is nothing on the partition to preserve at that point anyway.

- **CLI:** `parseDataSize` (cmd/gosd/build.go) accepts the keyword `expand` alongside byte sizes. `gosd run` is unchanged (it has no `--data-size` flag today).
- **Build:** pipeline threads a `DataExpand` bool into `internal/initcfg`'s config.json (the existing build-flag→boot-knob channel; see pipeline.go where config.json is written). `image.Spec` is untouched — `DataSizeBytes=0`, no partition 2.
- **First boot (gosd-init), BEFORE mounting /boot** — config.json lives in the initramfs so it's readable pre-mount, and with nothing mounted a partition-table reread can't hit EBUSY:
  1. If `dataExpand` and the boot device's MBR has no partition 2: identify the boot device (probe the existing candidate parents mmcblk0/mmcblk1/vda; verify partition 1 exists at the locked 16MiB offset, type 0x0C, as a sanity check that this is really a GoSD card).
  2. Read device size from `/sys/block/<dev>/size` — NOT BLKGETSIZE64 (broken on armv6, see gosd-fjio).
  3. Write MBR entry 2: type 0x0C, start 272MiB, end = device end (LBA count capped at uint32; align size down to a 4MiB boundary for flash friendliness). A single 512-byte sector write — no go-diskfs needed for this part.
  4. BLKRRPART (or BLKPG_ADD_PARTITION as fallback) so /dev/…p2 appears, then format it FAT32 labelled GOSD-DATA via `internal/diskfmt`.
  5. Continue the normal boot sequence — `mountData` works unchanged.
- **Idempotency / crash-safety:** the MBR itself is the marker. p2 present + FAT32 labelled GOSD-DATA → never touch (protects user data every later boot). p2 present but not a labelled FAT32 → power died between MBR write and format → reformat. p2 absent → do the expansion.
- **No room to expand** (card no bigger than the image; qemu-virt, where disk == image): log clearly, skip, /data gets today's read-only EROFS fallback. Not an error.

## Landmines / considerations

- **gosd-fjio** (todo): go-diskfs v1.9.3 reads BLKGETSIZE64 into a 4-byte int on GOARCH=arm — stack corruption + truncated sizes ≥4GiB. `diskfmt.FormatFAT32` goes through `diskfs.Open`, which hits that path, so gosd-fjio is a de-facto prerequisite for expand on pi-zero-w (or the formatter must be given an explicit size read from /sys). Since a plain `gosd build` builds ALL public boards, expand can't ship half-broken on armv6.
- **gosd-init binary size:** this imports `internal/diskfmt` (and thus go-diskfs) into gosd-init for the first time; the initramfs is RAM-resident. Measure the growth. The MBR-entry write itself needs none of it.
- **Format duration on big cards:** FAT size scales with the card (~2×128MiB of FAT writes on a 1TiB card at 32KiB clusters). Log a one-time "formatting data partition…" console line, and verify go-diskfs FAT32 correctness at large volume sizes against a sparse file.
- **docs/design/ab-updates.md** locks "updates never touch the partition table" — unaffected (this is first boot, not update), but its invariant deserves a sentence acknowledging first-boot expansion exists and completes long before any update could run.
- **Reflashing still wipes /data** — unchanged; a reflashed image has no p2 again and the next first boot recreates it.
- **Imager/catalog:** no change — Raspberry Pi OS does its own first-boot expansion under Imager, so this matches the flagship flow.
- **CI story:** qemu-virt can test the whole flow by enlarging the image file before boot (`qemu-img resize` / truncate) — first boot creates+formats+mounts, second boot leaves it alone.

## Todos

- [x] `parseDataSize`: accept `expand`; thread `DataExpand` through `pipeline.Options` → `initcfg` config.json
- [x] gosd-init expand step (device identification, MBR entry write, kernel partition registration, diskfmt format, idempotency rules above) — pure logic behind a seam with fake-driven tests, syscalls in platform_linux.go, per the house pattern. **Design refinement over the proposal above:** the step runs *after* the GOSD-BOOT mount, not before it — MountBootPartition's sentinel check has by then proven exactly which disk the system booted from, so only that disk can ever be expanded (the pre-mount design would have had to re-derive it heuristically, wrongly matching a stale eMMC image). The kernel learns of the partition via BLKPG_ADD_PARTITION (which works with p1 mounted) instead of a BLKRRPART reread (which would EBUSY).
- [x] Graceful no-free-space path (qemu-virt, exact-size cards): log + read-only /data fallback (floor: 64MiB of usable space)
- [x] Resolve gosd-fjio for armv6 device sizing — fixed properly in its own PR (#139); this branch is stacked on it, and the expand code's own device sizing uses the same lseek approach
- [x] Measure gosd-init size growth from importing diskfmt/go-diskfs: 7.18MB → 8.82MB (linux/arm64, plain `go build`), +1.65MB from go-diskfs and its transitive deps. Acceptable on 512MB boards; if it ever matters, a hand-rolled FAT32 formatter (as was done for exFAT) would claw it back
- [x] Integration test: build with `--data-size=expand`, read image back, assert no p2 + config.json flag (network-tripwire pattern, cmd/gosd/build_integration_test.go)
- [x] qemu boot test: enlarged qemu-virt image → first boot creates+formats+mounts /data; second boot doesn't touch it. Done twice over: locally on real qemu (both boots verified, plus MBR/format/marker durability across abrupt kills), and as a new CI job `qemu-expand-data` (scripts/boot-and-grep.sh boots the same enlarged image twice and greps the serial log)
- [ ] Bench sanity (sdwire): small image on a big card → /data spans the card, survives reboot
- [x] Docs: runtime.md persistent-storage section, `--data-size` help text, ab-updates.md note, COMPATIBILITY.md data footnote

## Findings along the way

- **go-diskfs fat32.Create corrupts volumes past ~256GiB** (uint16 sectors-per-FAT truncation) — filed as bean gosd-8kdm; expand caps its partition at 256GiB with a logged notice until that's fixed.
- **The write-fsync-rename pattern on /data is not durable until ~30s writeback expiry** (vfat `flush` flushes on close, and a rename involves no close) — found by killing qemu shortly after boot; filed as bean gosd-0nk4. Not expand-specific (identical on fixed-size images), and expand's own crash story doesn't depend on it: the MBR write is explicitly fsync'd, and an interrupted format is self-healed by the reformat-on-invalid rule.
