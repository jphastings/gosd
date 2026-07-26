---
# gosd-4ajn
title: 'usbwebsite: back the USB drive with the SD card''s GOSD-DATA partition when no eMMC'
status: completed
type: feature
created_at: 2026-07-26T07:58:02Z
updated_at: 2026-07-26T07:58:02Z
---

JP's locked intent (2026-07-25, his words): "change the usbwebsite example app
to use space on the SD card for the USB-gadget mountable data volume — that
way we can test with the rpi zero and rpi zero 2w." usbwebsite must work on
SD-only boards (pi-zero-w, pi-zero-2w) by backing the gadget's mass-storage
volume with SD-card space instead of (or as a fallback from) eMMC.

## Design (investigated 2026-07-26)

**Backing: the raw GOSD-DATA partition (partition 2 of the boot SD) as the
mass-storage LUN — not an image file on /data.** Why:

- `gosd build --data-size <n>` already creates it FAT32-formatted (label
  GOSD-DATA, MBR type 0x0C, `internal/image`), so no on-device format is ever
  needed — the SD path has **no destructive operation at all**, which is the
  strongest possible analog of gosd-4jn5/gosd-tdcc's consent semantics
  (nothing to consent to; nothing is ever reformatted).
- The kernel mounts it directly as vfat (gosd-init already proves this every
  boot), so serving needs no loop device (`CONFIG_BLK_DEV_LOOP` untested) and
  no Go FAT reader (which an image-file backing would force, since apps
  outside this module can't reach `internal/emmcfmt`).
- `gadget.MassStorage.Path` takes a block device as-is; a partition node
  exposes a "superfloppy" FAT volume every host OS mounts happily — exactly
  the shape the eMMC path already presents (whole-device FAT, no partition
  table).
- The host sees the volume label `GOSD-DATA` (plus gosd-init's `.gosd-data`
  marker file); the eMMC path keeps its `WEBSITE` label. Documented, not
  hidden.

**Ownership/lifecycle:** gosd-init owns the boot-time mount — it mounts
GOSD-DATA read-write at `/data` before the app starts (or a read-only tmpfs
when absent). usbwebsite makes its one-per-boot expose-or-mount decision on
top of that: to present the drive it `emmc.Unmount`s `/data` first (gadget
exclusivity — the host writes raw blocks), backs the LUN with the partition
device it read from `/proc/mounts`, and on any fallback (no computer
enumerated, gadget error) remounts the partition at `/data` and serves.
Host-write vs app-serve coherence is by mode exclusivity per boot, same as
the eMMC flow: switching modes means a power cycle, after which gosd-init
remounts and the app re-decides. If `/data` isn't vfat-mounted but the boot
disk has a p2 node (boot-time mount raced/failed, or a warm restart after
this app unmounted it), the app mounts it itself — derived from `/boot`'s
source device in `/proc/mounts`, mirroring gosd-init's bootDevices→
dataDevices p1→p2 relationship.

**eMMC precedence: eMMC preferred when fitted; SD is the ErrNoEMMC
fallback.** JP's phrasing frames SD as what makes the eMMC-less Pi Zeros
testable, and this keeps the existing (partly hardware-verified) Rockchip
behaviour byte-for-byte: FormatAndMount, the WEBSITE_WIPE_EMMC consent knob,
and the ErrRefusedFormat idle-with-guidance path are all untouched. Only
`errors.Is(err, emmc.ErrNoEMMC)` falls through to the SD path. No eMMC *and*
no GOSD-DATA partition → log the actionable fix (rebuild with
`--data-size`) and idle, exactly like today's no-eMMC behaviour (gosd-cgpr).

**Safety semantics:** the SD path never formats, relabels, or wipes
anything — it only ever mounts/unmounts the partition `gosd build` created
for app data and shares it over USB. WEBSITE_WIPE_EMMC continues to gate the
only destructive operation (eMMC reformat), unchanged.

## Kernel reality check (desk, 2026-07-26)

Both Pi Zeros' published kernel configs (`build/boards/pi-zero-w/kernel.config`,
`build/boards/pi-zero-2w/kernel.config`) already carry everything
f_mass_storage on dwc2 needs: `CONFIG_USB_DWC2=y` (+`_DUAL_ROLE`),
`CONFIG_USB_GADGET=y`, `CONFIG_USB_LIBCOMPOSITE=y`,
`CONFIG_USB_F_MASS_STORAGE=y`, `CONFIG_USB_CONFIGFS=y`,
`CONFIG_USB_CONFIGFS_MASS_STORAGE=y`, `CONFIG_CONFIGFS_FS=y` — matching the
proven gadget stack. `--usb-gadget` renders `dtoverlay=dwc2,dr_mode=peripheral`
into both boards' config.txt. **No kernel work needed in this PR.** Known gap
(already recorded in COMPATIBILITY.md's `[^usb-gadget]` footnote):
`CONFIG_USB_CONFIGFS_MASS_STORAGE` is only *incidental* on the Pi boards
(defconfig baseline; no fragment/`RequiredY` asserts it). Follow-up bean
recommendation: assert it in both Pi kernel fragments + kernelspec RequiredY
at the next fleet kernel tag bump (never a single-board bump).

## Todos

- [x] Refactor examples/usbwebsite: shared storage struct; eMMC path
      unchanged; SD GOSD-DATA fallback on ErrNoEMMC
- [x] Example-local platform seam (storage_linux.go / storage_other.go,
      sattrack precedent); pure mount-table/partition-derivation logic
      portable and unit-tested
- [x] README.md + package docstring: SD path, GOSD-DATA label on the host,
      per-board build commands incl. --data-size and the Pi Zeros
- [x] docs/runtime.md usbwebsite paragraph + COMPATIBILITY.md footnote text
- [x] Quality gates (go test/vet, gofmt, golangci-lint darwin+linux) and
      cross-compiles (GOOS=linux GOARCH=arm64; GOOS=linux GOARCH=arm GOARM=6)

## Bench validation (deferred — hardware session, tracked here unchecked)

- [ ] Pi Zero 2W: `gosd build --board pi-zero-2w --usb-gadget --data-size
      256MiB`; drive enumerates on a computer, files written land on
      GOSD-DATA; standalone boot serves them over HTTP
- [ ] Pi Zero W (armv6): same flow end-to-end, confirming the GOARM=6 build
      behaves identically

## Summary of Changes

- `examples/usbwebsite/main.go`: introduced a `storage` struct (mountpoint,
  block device, unmount/remount closures) that both backings satisfy.
  `claimStorage` keeps the eMMC path byte-for-byte in behaviour (consent
  knob, ErrRefusedFormat guidance-and-idle, unexpected-error exit) and falls
  through to `claimDataPartition` only on `errors.Is(err, emmc.ErrNoEMMC)`.
  The SD path finds the GOSD-DATA partition via `/proc/mounts` (`/data`'s
  device when gosd-init mounted it; otherwise partition 2 of `/boot`'s disk,
  mirroring gosd-init's own p1→p2 candidate lists), never formats or
  relabels anything, and idles with an actionable `--data-size` rebuild
  message when the image has no partition. `presentedAsDrive`/`remount` now
  operate on the storage struct; docstring and starter page reworded for
  both backings.
- `examples/usbwebsite/storage_linux.go` + `storage_other.go`: the example's
  only platform seam — `mountVFAT`/`unmountVFAT` via stdlib `syscall`, with
  gosd-init's exact GOSD-DATA options (nosuid/nodev + vfat `flush`); stubs
  keep macOS builds/tests green (sattrack's build-tag precedent).
- `examples/usbwebsite/main_test.go`: behavioral tests for the pure logic —
  `dataPartitionFromMounts` (mounted-by-init, derived-from-/boot, qemu vda,
  stacked-mount-wins, none-found) and `secondPartition`; kept
  `TestIsAffirmative`.
- `examples/usbwebsite/README.md`, `docs/runtime.md` (USB gadget section's
  usbwebsite paragraph), `COMPATIBILITY.md` (`[^no-emmc]` footnote): document
  the fallback, the GOSD-DATA host-visible label + `.gosd-data` marker, and
  per-board build commands (Pi Zeros need `--usb-gadget --data-size`).
- No public API changes: `gadget`/`emmc` untouched; the example uses only
  existing exported API plus stdlib.
- Follow-up recommendation (no bean filed yet): assert
  `CONFIG_USB_CONFIGFS_MASS_STORAGE=y` in both Pi kernel fragments +
  kernelspec RequiredY at the next fleet kernel tag bump — today it's only
  incidental from the defconfig baseline (already flagged in
  COMPATIBILITY.md's `[^usb-gadget]`).
