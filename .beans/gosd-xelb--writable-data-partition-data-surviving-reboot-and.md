---
# gosd-xelb
title: Writable data partition (/data) surviving reboot and reflash-of-app
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-05T05:52:59Z
parent: gosd-jge2
blocked_by:
    - gosd-cvzt
---

Apps need persistence (config, sensor logs). Everything else is RAM.

Locked v1: partition 2, FAT32, label GOSD-DATA, sized by builder flag `--data-size` (default 1GiB), created by the image writer (go-diskfs can format FAT32; there is no pure-Go mkfs.ext4 and the kernel cannot format — FAT is the honest v1). gosd-init mounts it rw at /data with flush,sync-friendly options; app gets GOSD_DATA=/data. Document limits plainly: no unix permissions/symlinks, not power-loss-robust — apps should write-rename and fsync.

- [x] Image writer: second partition support + format
- [x] gosd-init: mount rw with retry, create /data marker file on first boot
- [x] `gosd build` REUSE case: when flashing a new image version the data partition is recreated (wiped) — that is acceptable for v0.3 but document it loudly; the A/B update spike owns the preserve-across-updates story
- [ ] Pull-power torture test on hardware: 10 cycles while app writes once per second; record corruption findings here — this data decides whether v0.4 needs littlefs/f2fs

## Acceptance
Example app persists a counter across reboots on both boards; limits documented in the runtime contract page.

## Summary of Changes

Code half of this bean (pulled forward pre-hardware); the pull-power torture
test still needs real boards, so the bean stays in-progress.

- `internal/image`: `Spec.DataSizeBytes` adds an optional partition 2 —
  FAT32, label `GOSD-DATA`, MBR type 0x0C, starting immediately after
  GOSD-BOOT (byte 272MiB), size rounded down to whole sectors. Zero keeps
  the original single-partition layout byte-for-byte. The RawWrite overlap
  guard now also rejects writes into partition 2, and read-back tests cover
  both layouts.
- `gosd build --data-size` (default 1GiB; `--data-size=0` disables),
  accepting binary units (KiB/MiB/GiB/K/M/G) or plain bytes, threaded
  through `pipeline.Options.DataSizeBytes` → `image.Spec`. Verified
  empirically that go-diskfs writes the .img sparsely on APFS (1GiB data
  partition ≈ 22MiB on disk), so the integration test asserts the full
  default layout without disk/runtime cost.
- `cmd/gosd-init`: mounts `/dev/mmcblk?p2` read-write at `/data` (vfat
  `flush` option, nosuid/nodev) with the same retry/candidate pattern as
  /boot, but with a fast path: since this runs after the boot mount
  succeeded the card is already probed, so all-candidates-ENOENT is
  reported immediately as "no data partition" instead of burning the 10s
  timeout. Missing/unmountable partition is never fatal — the app just
  gets no `GOSD_DATA`. On success gosd-init creates `/data/.gosd-data` on
  first boot and exports `GOSD_DATA=/data`. Fake-driven tests per the boot
  package pattern.
- Docs: `docs/runtime.md` gains a "Persistent storage: /data" section
  (FAT32 limits, not power-loss-robust, write-rename+fsync guidance, and a
  loud "reflash wipes GOSD-DATA in v0.3; app-slot updates won't");
  `docs/design/ab-updates.md` §0 now states explicitly that GOSD-DATA
  survives app-slot updates.
- `examples/hello`: persists a boot counter to `GOSD_DATA` using the
  documented write-temp/fsync/rename pattern; no-op (and no failure) when
  `GOSD_DATA` is unset. Stdlib only.
