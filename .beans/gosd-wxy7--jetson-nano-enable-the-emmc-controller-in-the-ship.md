---
# gosd-wxy7
title: 'Jetson Nano: enable the eMMC controller in the shipped device tree'
status: todo
type: task
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-07T20:59:10Z
parent: gosd-72je
---

The board targets the eMMC module only (epic gosd-72je, decided 2026-11-07). On the bench, GoSD's kernel boots from the eMMC's `APP` partition, then gosd-init can't mount its boot partition (`tried /dev/mmcblk0p1, /dev/mmcblk1p1, /dev/vda1`) and reboots. Mainline `tegra210-p3450-0000.dts` describes the SD-card dev kit, whose module has no eMMC, so it leaves SDMMC4 (`mmc@700b0600`, the eMMC) disabled.

Ship a kernel-build DTS patch under `build/boards/jetson-nano/kernel/patches/`, as the Rockchip boards do:

- [ ] Enable SDMMC4: 8-bit, non-removable, with its supplies and speed modes taken from NVIDIA's `tegra210-p3448-0002-p3449-0000-b00` DTS (NVIDIA's CBoot runs it in "ddr50 mode").
- [ ] Disable SDMMC1, the dev kit's SD slot. It isn't on the module, so nothing should probe an empty socket.
- [ ] Pin the `mmcblk` numbering with `aliases`, so `APP` is a predictable `mmcblkNp1`. gosd-init only tries `mmcblk0p1`, `mmcblk1p1` and `vda1`.
- [ ] Check what gosd-init's data candidates (`mmcblk{0,1}p2`) then hit. That's NVIDIA's raw `DTB` partition, so the mount should fail harmlessly, but confirm it. `/data` belongs on the NVMe (see the NVMe data bean).
- [ ] Do the artifacts release: tag first, bump second.
- [ ] Bench: gosd-init mounts the boot partition and reads its config tree.
