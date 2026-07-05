---
# gosd-5wm0
title: 'qemu-virt kernel: virtio arm64 build'
status: completed
type: task
priority: normal
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T08:04:00Z
parent: gosd-c54j
---

Mirror build/boards/*/: Dockerized build.sh, pinned mainline tag (same 6.18.x as the Radxa for config reuse), defconfig + committed fragment, CONFIG_MODULES=n, everything =y: the standard GoSD baseline (initramfs RD_ZSTD, VFAT_FS+NLS, devtmpfs, IPv4/6, AF_PACKET) plus CONFIG_VIRTIO_BLK, CONFIG_VIRTIO_NET, CONFIG_VIRTIO_PCI, CONFIG_VIRTIO_MMIO, CONFIG_SERIAL_AMBA_PL011 (qemu-virt console ttyAMA0), CONFIG_RTC_DRV_PL031 (virt has an RTC — harmless, lets qemu boots have sane time). No dtb output (qemu -M virt synthesizes its own). Cut everything hardware-specific.

- [x] build.sh + fragment + committed generated kernel.config (provenance header)
- [x] VALIDATE BY BOOTING: inside a Linux container (apt qemu-system-arm), boot the built Image with a minimal initramfs and confirm console output reaches userspace — record the invocation in the bean
- [x] Add qemu-virt to build-artifacts.yml packaging

## Acceptance
Kernel boots to userspace under qemu-system-aarch64 -M virt; config committed; artifact packaging covers it.

## Summary of Changes

Added `build/boards/qemu-virt/kernel/` mirroring the Radxa Zero 3E kernel
pattern (Dockerized `build.sh` + `docker-build.sh`, debian:bookworm +
`aarch64-linux-gnu-` cross toolchain): pinned to the same mainline stable
tag as Radxa (`v6.18.37`), `make ARCH=arm64 defconfig` + `kernel-fragment.config`
merged via `merge_config.sh`, `CONFIG_MODULES=n`. Fragment adds
CONFIG_VIRTIO_BLK/NET/PCI/MMIO, CONFIG_PCI + CONFIG_PCI_HOST_GENERIC,
CONFIG_SERIAL_AMBA_PL011(_CONSOLE), CONFIG_RTC_DRV_PL031, and explicit cuts
for CONFIG_ARCH_ROCKCHIP/BCM*, CONFIG_WLAN/CFG80211, CONFIG_SOUND/SND,
CONFIG_DRM/FB, CONFIG_MEDIA_SUPPORT, CONFIG_BT. `docker-build.sh` asserts
both that every required option survived `olddefconfig` and that every cut
option stayed off, failing loudly on drift. Output is `Image` only (no
DTB — qemu -M virt synthesizes its own). Committed `kernel.config` carries
a provenance header (source repo/tag/generation method).

### Boot validation (real build, not simulated)

Built the kernel via `./build.sh` (Docker Desktop, native linux/arm64 on
this Apple Silicon host — no cross-arch emulation needed), producing a
61MiB `Image`. All `docker-build.sh` required/forbidden config assertions
passed.

Booted it inside a `debian:bookworm` container (`apt-get install
qemu-system-arm`, giving `qemu-system-aarch64` 7.2.22) with a throwaway
5-line Go `/init` (cross-compiled `GOOS=linux GOARCH=arm64
CGO_ENABLED=0`, packed into a cpio.zst via the repo's own
`internal/initramfs.Build`) that mounts procfs/sysfs, prints a marker,
and reports `/proc/uptime`, `/proc/partitions`, and `/sys/class/net`
contents:

```
docker run --rm -v "$(pwd):/work:ro" docker.io/library/debian:bookworm bash -c '
  apt-get update -qq && apt-get install -y -qq --no-install-recommends qemu-system-arm
  timeout 60 qemu-system-aarch64 -M virt -cpu cortex-a53 -m 512 -nographic -nic none \
    -kernel /work/Image -initrd /work/initramfs.cpio.zst \
    -drive if=none,file=/tmp/dummy-disk.img,format=raw,id=hd0 \
    -device virtio-blk-pci,drive=hd0,romfile= \
    -netdev user,id=n0 -device virtio-net-pci,netdev=n0,romfile= \
    -append "console=ttyAMA0"'
```

(`romfile=` empty is needed on this qemu/machine combo — without it,
virtio-pci device models look for `efi-virtio.rom`/similar PXE option
ROMs that direct `-kernel` boots never load, and qemu refuses to start.)

Console output confirmed the whole chain: PL011 console live from the
first boot line, PL031 RTC registered and set the system clock, PCI host
bridge (`pci-host-generic`) enumerated, `/init` ran as PID 1, and:

```
GOSD-QEMU-VIRT-BOOT-OK
GOSD-QEMU-VIRT-UPTIME: 1.39 0.00
GOSD-QEMU-VIRT-PARTITIONS:
major minor  #blocks  name
 253        0       4096 vda
GOSD-QEMU-VIRT-NET-IFACES: [eth0 lo sit0]
GOSD-QEMU-VIRT-DONE
```

`vda` is the attached virtio-blk drive; `eth0` is virtio-net — both
transports (PCI, since `romfile=` forced PCI-attached virtio here) proven
live, not just compiled in. Total container-to-marker time was ~20s
(dominated by `apt-get install` in a cold container with no image
caching); guest-visible boot-to-userspace time was ~1.4s. TCG emulation
was not a bottleneck: host and guest share the arm64 ISA (Apple Silicon
host, cortex-a53 guest), so this never needed slow cross-ISA translation
— unlike an amd64 CI runner, which will genuinely emulate arm64 and should
be expected to run slower (untested here; worth confirming timing once
gosd-27lz's CI job lands).

### CI packaging

Added a `qemu-virt-kernel` job to `.github/workflows/build-artifacts.yml`
(same shape as the other kernel jobs), wired its `Image` into
`package-and-release` as a fourth `staging/qemu-virt/` directory with its
own `source.json` provenance, and added `qemu-virt.tar.zst` to the
published release's file list. Documented in `docs/artifacts.md` as an
internal-only board: packaged through the same pipeline so
`internal/artifacts.EnsureBoard`/`--board=qemu-virt` can fetch it like any
other board, but never mentioned in end-user-facing instructions.

### Deviations from the bean

None — every required config option, the boot validation, and the CI
packaging landed as specified. `internal/boards` registration
(`--board=qemu-virt` wiring, gosd-init device candidate lists) is
correctly out of scope here per the sibling bean gosd-2v40, which is
blocked on this one.
