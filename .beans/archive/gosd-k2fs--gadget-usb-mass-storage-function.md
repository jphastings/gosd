---
# gosd-k2fs
title: 'gadget: USB mass-storage function'
status: completed
type: feature
priority: normal
created_at: 2026-07-13T13:19:54Z
updated_at: 2026-07-13T15:27:19Z
parent: gosd-jge2
---

Extend the public `gadget/` package (currently serial/Ethernet functions) with a USB mass-storage function (configfs f_mass_storage): LUN backed by a block-device path, read-only flag, removable flag. Follows the package's existing function-type shape. Driving use case: betamin exposes its unmounted NVMe SSD partition to a host computer for drag-and-drop video transfer (exFAT, host-native) — MSC mode and local mount are mutually exclusive by app design.

## Locked decisions

- Public API surface (`gadget/` is semver-relevant, v0.3) — note the minor-version implication in the PR.
- Pure logic behind the interface seam with fake-driven tests passing on macOS; real configfs writes in platform_linux.go (repo convention).
- Kernel dependency: boards need `CONFIG_USB_CONFIGFS_MASS_STORAGE=y`. rock-4se gets it in its initial kernel (epic gosd-cuym); other boards gain it at the next fleet kernel tag bump — track that follow-up here, do NOT bump a single board in isolation.
- COMPATIBILITY.md: extend the USB gadget row/footnote accordingly.

## Todo

- [x] gadget/ mass-storage function type + configfs wiring (platform_linux.go + stubs)
- [x] Fake-driven tests
- [x] COMPATIBILITY.md gadget footnote update
- [x] Follow-up note/bean for fleet kernel fragment addition at next tag bump (recorded as the Follow-up section below)

## Follow-up: assert CONFIG_USB_CONFIGFS_MASS_STORAGE=y at the next fleet kernel tag bump

Every current board's recorded published `kernel.config` already carries
`CONFIG_USB_CONFIGFS_MASS_STORAGE=y`, but only incidentally — it's inherited
from the defconfig baseline and asserted by no kernel fragment and no
`internal/kernelspec` `RequiredY` list. At the next fleet kernel tag bump
(all boards together, never one in isolation), add it explicitly to the Pi
boards' `kernel.fragment`s (RequiredY derives from those) and to the
Rockchip/qemu-virt fragments + hand-maintained RequiredY lists, so the
gadget mass-storage dependency is guaranteed rather than incidental.
rock-4se needs nothing: its initial stock kernel asserts it already
(epic gosd-cuym, bean gosd-iosp). No kernel fragment was changed in this
bean's PR, per the locked decision.

## Note on the seam shape

The locked decision's wording names `platform_linux.go` + stubs (the generic
repo convention), but the merged `gadget/` package's reviewed seam is the
unexported `writableFS` interface (`osFS` in production, `fakeFS` in tests)
with no build-tagged files — all configfs writes already flow through it, and
`Function.Create(fsys writableFS, dir string)` is the hook function types
implement. `MassStorage` follows that existing shape exactly (its attribute
writes all go through `writableFS`; tests are fake-driven and pass on macOS),
rather than introducing a new build-tagged layer into a package that has
none. The decision's intent — pure logic behind a seam, fakes on macOS, real
writes isolated — is fully met.

## Summary of Changes

Added `gadget.MassStorage`, a USB mass-storage `Function` (configfs
`f_mass_storage`) exposing one LUN backed by a block device or disk-image file,
with `ReadOnly` and `Removable` flags. It implements the package's existing
`Function` interface (`Name`/`Create`) exactly like `ACM`, writing its LUN
attributes through the `writableFS` seam — flags before `file`, matching the
kernel's refusal to change a LUN's flags once its backing file is open.

- `gadget/massstorage.go`: the function type + attribute writes.
- `gadget/massstorage_test.go` + `gadget/fakes_test.go`: fake-driven tests
  (pass on macOS) covering attribute values, flag defaults, the load-bearing
  write order, the empty-`Path` error, and clean teardown. The fake now models
  configfs default groups, so the kernel-owned `lun.0` (auto-created with the
  function dir, un-removable directly, cascade-removed with its parent) is
  faithfully represented rather than papered over.
- `COMPATIBILITY.md` + `docs/runtime.md`: documented mass storage, the
  `CONFIG_USB_CONFIGFS_MASS_STORAGE=y` kernel dependency, and the expose-or-mount
  (never both) constraint.

Extends the semver-relevant public `gadget/` package (v0.3) — additive, so a
minor bump. No kernel fragment changed (locked decision); the guaranteed-config
follow-up is recorded above.
