---
# gosd-fnh8
title: 'gadget: MassStorage accepts a currently-mounted backing device — the documented expose-or-mount-never-both rule is unenforced'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:58:52Z
updated_at: 2026-08-02T00:00:00Z
---

Found by review sweep `gosd-fuxs` (storage area), verified.

MassStorage.Create (gadget/massstorage.go:36-54) validates only that Path
is non-empty and writes it into lun.0/file; nothing consults
/proc/mounts. The only guard is prose in three doc comments — and the
dangerous call is the docs' own two snippets concatenated:
FormatAndMount returns res.BlockDevice, and handing that to MassStorage
while it's still mounted has the board's vfat page cache and the USB
host's raw writes corrupting the volume with no error anywhere.

**Failure scenario:** developer follows the disk + gadget examples,
forgets the Unmount line → intermittent corruption of the shared volume.

**Fix:** MassStorage.Create (or Apply) rejects a Path present in — or a
partition of a device present in — blockmount.MountedSources(), with an
actionable error naming the mountpoint and the Unmount step. The check is
a /proc/mounts read blockmount already implements. Adjacent: gosd-k2fs
(mass-storage scope, locked) — this adds enforcement, not scope.

## Summary of Changes

`MassStorage.Create` now refuses a `Path` that is mounted, a partition of a
mounted device, or the parent device of a mounted partition, before writing
any LUN attribute — the error names the mountpoint and tells the caller to
`Unmount` it first.

- **Where the check lives:** inside `Create`, not a separate `Apply`-level
  step. `Create` already runs mid-`materialize()`, and PR #153's unwind
  machinery (`failApply`/`removeConfigfsTree`) unwinds *any* `materialize()`
  failure uniformly regardless of which step raised it — so rejecting here
  gets a clean unwind for free, the same way a `Function.Create` that fails
  to write an attribute already does. No new unwind path was needed.
- **Seam:** an unexported `mountedTargets func() (map[string]string, error)`
  field on `MassStorage` (source device -> mountpoint), matching the
  package's existing fake-driven-test convention rather than introducing a
  new kind of dependency. Production default lives behind the package's own
  `platform_linux.go`/`platform_other.go` split (new files — the package had
  none before, since everything previously went through `writableFS`), and
  calls a new, purely additive `blockmount.MountedTargets()` (parallel to
  the existing `MountedSources()`, kept alongside its mountpoint so the
  error can name it) — no change to `blockmount.Run` or its `Deps`, so no
  expected conflict with gosd-45bv's parallel work there.
- **Containment logic:** a small pure, portable helper
  (`relatedDevicePaths`/`isPartitionOf` in `gadget/massstorage.go`, no build
  tag) matches Linux's own partition-naming convention (bare digit suffix
  for a parent ending in a letter — `sda`→`sda1`; `p`-prefixed digit suffix
  for a parent ending in a digit — `nvme0n1`→`nvme0n1p1`,
  `mmcblk0`→`mmcblk0p1`) against every currently-mounted source, covering
  all three containment directions (exact match, `Path` is a mounted
  partition's parent, `Path` is a partition of a mounted whole device) in
  both directions without needing a `/sys/block` topology read — only the
  `/proc/mounts` data the bean called for. Guarded to `/dev/*` paths only,
  so a disk-image-file `Path` (e.g. `/data/image.bin`) is never
  false-positived against a coincidentally-named sibling file.
- **Tests** (`gadget/massstorage_test.go`): all three containment
  directions rejected (table-driven, both `sdX`/`nvme` naming conventions),
  an unrelated mount never blocking `Path`, the mount-check reader's own
  error propagating actionably, and a direct table test of
  `relatedDevicePaths` covering the naming edge cases (eMMC hardware
  partitions, unrelated devices, non-`/dev` paths). Plus the exact scenario
  the bean exists to catch: mounting a device and handing it straight to
  `MassStorage` without `Unmount` fails loudly and unwinds every bit of
  gadget state (`assertNoGadgetState`), and the same sequence *with*
  `Unmount` succeeds. Existing tests updated to inject an empty
  `mountedTargets` fake now that the check is unconditional.
- `docs/runtime.md`'s USB gadget section now documents the enforcement
  (mountpoint-naming refusal + unwind) instead of describing
  expose-or-mount-never-both as prose-only convention.
