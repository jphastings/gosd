---
# gosd-g0of
title: Bump internal/artifacts.Version to v0.10.2
status: completed
type: task
priority: normal
created_at: 2026-08-17T08:18:58Z
updated_at: 2026-08-17T08:53:09Z
parent: gosd-h1wv
---

The bump half of tag-first/bump-second for `artifacts/v0.10.2`. Until this
lands, **none of the Cubie A5E fixes reach real builds**: `gosd build` still
downloads v0.10.0, so a 1GB board still gets an image that halts in SPL.

## What v0.10.2 carries for cubie-a5e

- **1GB DRAM calibration** (bean gosd-84b8, PR #292) — without it the released
  image cannot boot the 1GB variant at all. This is the reason the bump matters.
- **No preboot USB scan** (bean gosd-uj4l, PR #298) — U-Boot phase 9.05s → 4.50s,
  measured over 5 clean power cycles.
- **A USB-gadget variant DTB** (bean gosd-3io0, PR #301) — built and published,
  but NOT yet consumed by the board profile; that is a separate follow-up
  gated on proving enumeration.

## Verification required (CLAUDE.md's three ways)

- [x] Clean-machine build: fresh `HOME`, no `--board`/`--artifacts-dir`, all
      public images built from a real download
- [x] Offline re-run: dead proxy, succeeds entirely from cache
- [x] Content spot-check: the released cubie-a5e tarball really carries the
      changes — `dtc -I dtb -O dts` shows `ehci0`/`ohci0` disabled in the
      gadget DTB, and the U-Boot binary has no `preboot=usb start` in its
      built-in environment
- [x] Bench re-verify on the 1GB board: a plain `gosd build --board cubie-a5e`
      with no `--artifacts-dir` boots, which is the end-to-end proof that the
      DRAM fix reaches users

## Todos

- [x] Bump the constant and its doc comment (note what v0.10.2 carries)
- [x] Run the three verifications above and record them here
- [x] Bench-boot an image built from the real release


## Summary of Changes

`internal/artifacts.Version` bumped v0.10.0 → **v0.10.2**, so the Cubie A5E
fixes finally reach ordinary `gosd build` runs. Its doc comment now records
what v0.10.1 and v0.10.2 each carry, including that the gadget DTB is
published but deliberately not yet consumed.

### Verified four ways

1. **Clean-machine build** — fresh `HOME`, no `--board`, no `--artifacts-dir`:
   all seven public board images built from real downloads, and the cache
   under the fresh HOME holds `artifacts/v0.10.2`.
2. **Offline re-run** — same HOME, every proxy pointed at a dead port
   (`127.0.0.1:9`): the cubie-a5e build completed entirely from cache.
3. **Content spot-check** against the downloaded release tarball:
   `sun55i-a527-cubie-a5e-gadget.dtb` really has `ehci0`/`ohci0` **disabled**
   while the stock DTB has them `okay` and both keep `ehci1` (the Type-A
   port); the U-Boot binary contains **no** `preboot=usb start`; and the
   release manifest records all three fragments, `dram-1gb.config` included.
4. **Bench boot of an image built from the real release** (no
   `--artifacts-dir` anywhere):

```
U-Boot SPL 2026.04 (Aug 17 2026 - 08:14:22 +0000)     <- the CI-built binary
DRAM: 1024 MiB                                         <- the DRAM fix, live
Starting kernel ...
gosd hello: listening on [::]:80
[gosd] eth0: lease {192.168.1.201 ...}
[gosd] mdns: answering as hello.local on all up interfaces
```

**6.63s SPL→app**, zero USB-scan lines, and `ping hello.local` answers in
0.78ms. Both board fixes are now proven to reach users through the normal
download path, which is the thing the bump exists to establish.
