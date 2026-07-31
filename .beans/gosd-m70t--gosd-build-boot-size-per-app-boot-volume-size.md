---
# gosd-m70t
title: 'gosd build --boot-size: per-app boot volume size'
status: completed
type: feature
priority: normal
created_at: 2026-07-31T10:25:45Z
updated_at: 2026-07-31T10:32:47Z
blocked_by:
    - gosd-lirl
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §0.4). Parameterize internal/image's boot partition size: --boot-size flag (default 256MiB, today's constant), validated at flag-parse time (min: fits the payload — surface the current raw go-diskfs disk-full failure as an actionable error naming --boot-size; max: sane MBR/FAT32 bounds). The chosen size becomes the app's layout ABI: changing it in a later release erases GOSD-DATA on upgrade (documented; see the design's §2 grow/shrink analysis). Also: print boot-volume usage (payload bytes / size) at the end of every build so developers watch their headroom shrink across releases. Motivating case: Betamin's >1GB boot volume; also unblocks app-slot OTA (gosd-vxal) slot space for large apps.

## Summary of Changes

- **internal/image**: `bootPartitionOffsetBytes` (16MiB, the boot partition's
  *start*) stays a locked constant; its former sibling `bootPartitionSizeBytes`
  is now `Spec.BootSizeBytes` (per-`Write` call, zero means the exported
  `DefaultBootPartitionSizeBytes` = 256MiB). `computeLayout` derives
  `dataPartitionOffsetBytes`/`dataPartitionStartLBA`/`totalSizeBytes` from the
  resolved boot size instead of a compile-time constant, with the same
  negative/sub-sector/uint32-overflow guards the data partition already had.
  `Write` now returns `(WriteReport, error)`; `WriteReport` carries
  `BootPartitionSizeBytes` and `BootPartitionPayloadBytes` (sum of every
  `BootFiles` entry's content length) for the usage report. go-diskfs's bare
  `"no space left on device"` (from `filesystem/fat12.allocateSpace`, no
  exported sentinel) is now recognized by substring match and wrapped in the
  new `ErrBootPartitionFull`.
- **internal/pipeline**: `Options.BootSizeBytes` threads through to
  `image.Spec.BootSizeBytes`; `Assemble` now returns `(image.WriteReport, error)`.
- **cmd/gosd**: new repeatable-safe `--boot-size` flag on both `build` and
  `run` (default `"256MiB"`, pinned against `image.DefaultBootPartitionSizeBytes`
  by a test so the two can't drift). `parseDataSize`'s numeric core is
  factored into a shared `parseSizeBytes(flagName, s)`, reused by the new
  `parseBootSize`, which layers --boot-size's own bounds: minimum 1MiB (a
  structural typo-guard only — real payload fit is checked at build time,
  per the bean), maximum `diskfmt.MaxFAT32Bytes()` (same FAT32-formatter
  ceiling `--data-size` uses), and mandatory whole-MiB alignment (rejects
  with a rounded suggestion). A `pipeline.Assemble` error wrapping
  `image.ErrBootPartitionFull` is re-wrapped with actionable `--boot-size`
  guidance instead of surfacing go-diskfs's raw disk-full text. Every
  successful build prints a one-line `<board> boot volume: <used> / <size>
  used (<pct>%)` summary (`gosd build` per board, `gosd run` once).
- **Rockchip raw writes are unaffected**: idbloader (32768) and u-boot.itb
  (8388608) offsets, and the boot partition's 16MiB *start*, are independent
  of `--boot-size` — confirmed by grep (only `cmd/gosd-init/internal/dataexpand`
  still hardcodes the old 272MiB boundary, deliberately untouched here; see
  below) and by the existing Rockchip fake-artifacts integration tests, which
  pass unchanged.
- **Tests**: `internal/image` gained a layout test (non-default
  `BootSizeBytes` moves partition 2's offset, `WriteReport` values) and an
  `ErrBootPartitionFull` test. `cmd/gosd` gained `parseBootSize` unit tests
  (valid sizes, invalid input, too-small, misaligned, over the FAT32 ceiling)
  and two `build_integration_test.go` fixture tests: a non-default
  `--boot-size` + fixed `--data-size` build asserting partition 1's size and
  partition 2's offset read back from the image (plus the stderr usage
  line), and a `--boot-size=1MiB` build against the real cross-compiled
  hello/gosd-init payload asserting the actionable `ErrBootPartitionFull`
  refusal. All pre-existing tests (including the default-256MiB goldens)
  pass unchanged.
- **Not touched**: `docs/design/upgrade-path.md`, `COMPATIBILITY.md` (no row
  changed), and `cmd/gosd-init/internal/dataexpand` — its mirrored
  `dataPartitionStartLBA` constant still assumes the old fixed 272MiB
  boundary and is fixed by the parallel bean `gosd-lirl` (deriving the data
  offset from the flashed MBR instead). **This PR must merge after
  gosd-lirl's**: until dataexpand reads the MBR, a non-default `--boot-size`
  `--data-size=expand` image would have first boot create GOSD-DATA inside
  the boot partition.
- **Merge-time addition (bean gosd-e3e3 landed on main mid-task)**: PR #156
  added `diskfmt.LargestSelfConsistentFAT32Bytes`, trimming `--data-size` to
  the largest FAT32 volume go-diskfs lays out with an addressable FAT (at
  most two clusters less) — exactly the "internal/diskfmt's FAT32
  self-consistency helpers" this bean's body already named. Resolving the
  merge conflict in `computeLayout` applied the identical trim to the boot
  partition, so `--boot-size` gets the same self-consistency guarantee
  `--data-size` does. Verified by hand that the 256MiB default, and every
  boot/data size this PR's own tests use (32MiB, 128MiB, 4MiB, 1MiB), fall
  outside the trim's affected bands, so no existing golden churns; one size
  originally used in a new fixture (64MiB) does fall in an affected band and
  was swapped for 128MiB to keep that test an exact-equality check rather
  than a trim-aware one.
