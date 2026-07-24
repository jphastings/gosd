---
# gosd-pcwl
title: gosd-init can mount the wrong boot partition when eMMC precedes SD
status: completed
type: bug
priority: normal
created_at: 2026-07-24T06:38:28Z
updated_at: 2026-07-24T07:46:42Z
---

Found during NanoPi Zero2 eMMC-refit testing (gosd-odp7, 2026-07-24). With an eMMC fitted, the kernel enumerates it as mmcblk0 and the SD as mmcblk1. gosd-init's boot-partition probe walks candidates by device name (mmcblk0p1 first), so it probed the eMMC's first partition as GOSD-BOOT — kernel FAT driver rejected it ('FAT-fs (mmcblk0p1): bogus number of reserved sectors', retried ~3 times over ~0.8s) and the probe then fell through to the SD's mmcblk1p1, which mounted fine. Failed-safe ONLY because that eMMC partition isn't valid FAT: any eMMC whose p1 IS a valid FAT filesystem (vendor-shipped image, previously-flashed GoSD image) would be mounted as /boot instead of the SD the user just flashed — stale gosd.toml, wrong app config, very confusing failures.

Fix direction: identify GOSD-BOOT by FAT volume label (the partitions are labelled GOSD-BOOT by the image builder) rather than accepting the first mountable FAT by device-name order — check the label before committing, or enumerate by-label via the kernel's vfat label support in gosd-init's probe seam. Also consider logging the DEVICE the boot partition was mounted from ('boot partition mounted at /boot' currently omits the source, which slowed diagnosis).

Repro on bench: NanoPi Zero2 + any eMMC with a valid-FAT first partition + GoSD SD card; without the fix /boot comes from eMMC. Runtime code: gosd-init boot-partition probe (see the mmcblk0p1/mmcblk1p1/vda1 candidate loop in the boot log). Tests: fake-driven per the gosd-init platform seam convention.

## Summary of Changes

Mechanism chosen: **(b) sentinel file**, not (a) FAT volume label — despite the
bean's original fix direction suggesting the label.

Rationale: statfs doesn't expose FAT volume labels, so (a) would need a new
raw-block-device reader that opens the candidate ahead of/alongside the
mount, seeks to the FAT32 BPB's BS_VolLab field (offset 0x47, 11 bytes,
space-padded), and parses it by hand — extra binary-format code to get right
and keep right. (b) reuses the mount gosd-init already performs: after a
candidate mounts as FAT, check for gosd.toml at its root before accepting it;
unmount and keep probing if it's absent. gosd.toml is written onto every
board's boot partition unconditionally by internal/pipeline.Assemble (see the
comment there: "gosd.toml is common to every board, unlike
config.txt/extlinux.conf"), so it's present from the very first boot on every
board, which is exactly what the sentinel needs. This also verifies the thing
gosd-init actually cares about (is this readable as a GoSD boot partition) as
directly as possible, rather than a proxy signal. Cost: an extra mount+unmount
round-trip only in the rare case a wrong candidate is itself valid FAT
(the common case — an SD alone, no eMMC — costs nothing extra); negligible
against the existing 250ms retry cadence.

Implementation:
- `Mounter` (cmd/gosd-init/internal/boot/interfaces.go) gained `Unmount(target
  string) error`; real impl in platform_linux.go wraps `unix.Unmount`,
  platform_other.go stubs it like the rest of the platform seam.
- `MountBootPartition` (mounts.go) now takes a `pathExists func(path string)
  bool` parameter and returns `(device string, err error)` instead of just
  `error`. After a candidate mounts, it checks `pathExists(target +
  "/gosd.toml")`; on failure it unmounts and moves to the next candidate
  within the same retry round/timeout budget as an outright mount failure.
- `Deps.PathExists` (sequence.go) is nil-checked like the package's other
  optional deps (ReadGosdToml, EnsureDataMountpoint, ...) — Run() defaults it
  to "always true" when unset, so every pre-existing test that doesn't care
  about the sentinel check keeps passing unmodified.
- main.go wires the real `PathExists` to a plain `os.Stat`-backed closure
  (same pattern as EnsureDataMountpoint/EnsureDataMarker — plain file I/O,
  not a privileged syscall, so it doesn't need a platform_linux.go split).
- The boot-partition-mounted log line now includes the source device:
  `boot partition mounted at /boot from /dev/mmcblk1p1`.

Tests (cmd/gosd-init/internal/boot/mounts_test.go, sequence_test.go):
- TestMountBootPartitionSkipsCandidateMissingGosdBootSentinel: the exact
  hardware scenario — first candidate mounts as FAT but lacks gosd.toml,
  second candidate has it; asserts the second device is returned and exactly
  one unmount happened.
- TestMountBootPartitionAcceptsSingleCandidateWithSentinelPresent: the normal
  single-candidate (no eMMC) path still works, with zero unmounts.
- TestRunHappyPathOrchestratesTheBootSequence extended to assert the console
  log line names the source device.
- Existing MountBootPartition tests updated for the new signature/return
  value; fakeMounter gained Unmount tracking (unmounts, unmountsFor).
