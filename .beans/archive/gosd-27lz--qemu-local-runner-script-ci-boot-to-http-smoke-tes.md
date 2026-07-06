---
# gosd-27lz
title: qemu local runner script + CI boot-to-HTTP smoke test
status: completed
type: task
priority: normal
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T15:27:54Z
parent: gosd-c54j
blocked_by:
    - gosd-2v40
---

The payoff task.
- [x] scripts/qemu-run.sh: takes an app .img built with --board=qemu-virt, extracts Image+initramfs from the FAT partition (mtools or go-diskfs helper — no root), launches qemu-system-aarch64 -M virt -cpu cortex-a53 -m 512 with the .img as virtio disk, user networking with hostfwd tcp:8080→:80, serial on stdio. Document in docs/runtime.md (developer section): brew install qemu / apt qemu-system-arm.
- [x] CI job in ci.yml: build examples/hello --board=qemu-virt (real qemu kernel from the artifact cache/release), boot it headless, poll http://localhost:8080 until the hello response arrives (timeout ~120s TCG), fail with the captured serial log on timeout. This is the first time gosd-init ever runs as PID 1 in CI — expect to file bug beans for what it flushes out; list them here.
- [x] Note the future `gosd run` command as a follow-up bean once this proves stable (filed as gosd-wnsj)

## Acceptance
CI red/green reflects real boot success; a developer can run one script locally and curl their app.


## Summary of Changes

- `scripts/qemu-run.sh`: takes a `--board=qemu-virt` .img, extracts `Image` +
  `initramfs.cpio.zst` from the FAT boot partition without root via the new
  `internal/cmd/imgextract` (go-diskfs, read-only open; no mtools needed), and
  execs `qemu-system-aarch64 -M virt -cpu cortex-a53 -m 512 -nographic` with
  serial on stdio, the .img as a virtio-blk disk, and user networking with
  hostfwd tcp 8080→80. Device flags are the validated explicit form from
  gosd-5wm0 (`-device virtio-blk-pci,drive=hd0,romfile=` /
  `-device virtio-net-pci,netdev=n0,romfile=`) — `romfile=` avoids qemu
  refusing to start when PXE option ROMs aren't shipped; the bean's
  `if=virtio` / `virtio-net-device` (mmio) forms were also boot-tested and
  work, the PCI form was kept to match the invocation validated in gosd-5wm0.
- `internal/cmd/imgextract`: tiny go-run-able helper copying every file at
  the FAT boot partition root of an image to a directory.
- `docs/runtime.md`: new "Testing your app under qemu (no hardware needed)"
  developer section (brew install qemu / apt qemu-system-arm, usage, what to
  expect on the serial console).
- `.github/workflows/ci.yml`: new `qemu-boot` job — builds examples/hello
  with a plain `gosd build --board=qemu-virt` (NO --artifacts-dir: the real
  internal/artifacts release-download path), caches `~/.cache/gosd/artifacts`
  keyed on `hashFiles('internal/artifacts/artifacts.go')` (which declares the
  pinned Version), installs qemu-system-arm, boots via scripts/qemu-run.sh,
  polls http://localhost:8080 for up to 180s (amd64 TCG emulation headroom)
  until the response contains `host=ci-qemu-hello`, and dumps the captured
  serial log into the CI output on failure.

### Runtime bugs found and fixed (first real PID-1 boot)

1. **Initramfs contained no mount-point directories — gosd-init could never
   boot.** `boot.mountEarly`'s very first mount (`devtmpfs` on `/dev`, then
   proc/sysfs/tmpfs) and `MountBootPartition`'s `/boot` target all fail with
   ENOENT because `internal/initramfs` only wrote `/init`, `/app`,
   `/etc/gosd/config.json`, and firmware — the kernel does not create any
   directories in rootfs itself. Every board (not just qemu-virt) was
   affected; first observed as `[gosd] fatal: mounting early filesystems
   failed: mounting proc at /proc: no such file or directory; rebooting in
   5s`. Fixed by adding `Spec.Dirs` to `internal/initramfs` (explicit empty
   directories, deduplicated against file-implied parents) and having
   `internal/pipeline` pass `/dev /proc /sys /run /boot`.
2. **netup never brought up an interface that starts administratively
   down — networking never came up, HTTP unreachable.** `Links.Watch`
   (netlink, ListExisting) reports virtio-net's eth0 with OperState down at
   boot; there is no udev/NetworkManager to run `ip link set up`, and
   `handleLinkEvent` only reacted to up-transitions, so `SetUp` was never
   called and DHCP never started. Fixed: a down event for a wired interface
   with no running DHCP loop now calls `Links.SetUp`; the kernel's resulting
   OperUp event then starts DHCP through the existing path. Regression test
   `TestRunBringsUpAFreshlyDiscoveredDownInterface`. This affects real
   boards identically (eth-equipped Radxa Zero 3E), not just qemu.

### Validation

Full local end-to-end on this host (arm64 macOS): kernel built via
`build/boards/qemu-virt/kernel/build.sh`, `gosd build ./examples/hello
--board=qemu-virt --artifacts-dir <built kernel>`, booted with
scripts/qemu-run.sh — gosd-init reaches /app in ~3s guest time, DHCP lease
on eth0, hello answers on http://localhost:8080 (~4s wall-clock from qemu
start, boot counter persists across reboots via /dev/vda2). Repeated inside
a linux/arm64 golang:1.26-bookworm container with apt-installed
qemu-system-arm (7.2.x, the CI package): HTTP success on the 3rd 1s poll.

### Observations (filed, not fixed here)

- mDNS conflict detection false-positives against the device's own address
  under qemu user networking (log-only): bug bean gosd-90ir.
- `gosd run` follow-up: gosd-wnsj.
- The `artifacts/v0.1.0` release was still unpublished at PR time, so the
  qemu-boot CI job fails at its build step (with the actionable
  release-missing error from internal/artifacts) until that release lands;
  everything after that step is proven by the local container run.
