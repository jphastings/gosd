---
# gosd-5wm0
title: 'qemu-virt kernel: virtio arm64 build'
status: todo
type: task
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T07:07:13Z
parent: gosd-c54j
---

Mirror build/boards/*/: Dockerized build.sh, pinned mainline tag (same 6.18.x as the Radxa for config reuse), defconfig + committed fragment, CONFIG_MODULES=n, everything =y: the standard GoSD baseline (initramfs RD_ZSTD, VFAT_FS+NLS, devtmpfs, IPv4/6, AF_PACKET) plus CONFIG_VIRTIO_BLK, CONFIG_VIRTIO_NET, CONFIG_VIRTIO_PCI, CONFIG_VIRTIO_MMIO, CONFIG_SERIAL_AMBA_PL011 (qemu-virt console ttyAMA0), CONFIG_RTC_DRV_PL031 (virt has an RTC — harmless, lets qemu boots have sane time). No dtb output (qemu -M virt synthesizes its own). Cut everything hardware-specific.

- [ ] build.sh + fragment + committed generated kernel.config (provenance header)
- [ ] VALIDATE BY BOOTING: inside a Linux container (apt qemu-system-arm), boot the built Image with a minimal initramfs and confirm console output reaches userspace — record the invocation in the bean
- [ ] Add qemu-virt to build-artifacts.yml packaging

## Acceptance
Kernel boots to userspace under qemu-system-aarch64 -M virt; config committed; artifact packaging covers it.
