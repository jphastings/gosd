---
# gosd-wxy7
title: 'Jetson Nano: enable the eMMC controller in the shipped device tree'
status: in-progress
type: task
priority: normal
created_at: 2026-11-07T20:59:10Z
updated_at: 2026-11-07T21:46:01Z
parent: gosd-72je
---

The board targets the eMMC module only (epic gosd-72je, decided 2026-11-07). On the bench, GoSD's kernel boots from the eMMC's `APP` partition, then gosd-init can't mount its boot partition (`tried /dev/mmcblk0p1, /dev/mmcblk1p1, /dev/vda1`) and reboots. Mainline `tegra210-p3450-0000.dts` describes the SD-card dev kit, whose module has no eMMC, so it leaves SDMMC4 (`mmc@700b0600`, the eMMC) disabled.

Ship a kernel-build DTS patch under `build/boards/jetson-nano/kernel/patches/`, as the Rockchip boards do:

- [x] Enable SDMMC4: 8-bit, non-removable, with its supplies and speed modes taken from NVIDIA's `tegra210-p3448-0002-p3449-0000-b00` DTS (NVIDIA's CBoot runs it in "ddr50 mode").
- [x] Disable SDMMC1, the dev kit's SD slot. It isn't on the module, so nothing should probe an empty socket.
- [x] Pin the `mmcblk` numbering with `aliases`, so `APP` is a predictable `mmcblkNp1`. gosd-init only tries `mmcblk0p1`, `mmcblk1p1` and `vda1`.
- [x] Check what gosd-init's data candidates (`mmcblk{0,1}p2`) then hit. That's NVIDIA's raw `DTB` partition, so the mount should fail harmlessly, but confirm it. `/data` belongs on the NVMe (see the NVMe data bean).
- [ ] Do the artifacts release: tag first, bump second.
- [ ] Bench: gosd-init mounts the boot partition and reads its config tree.

## Implementation notes

`build/boards/jetson-nano/kernel/patches/0001-enable-emmc.patch`, written against Linux v6.18.60's `tegra210-p3450-0000.dts`. It applies with `patch -p1 --fuzz=0` and `git apply --check`, and the patched DTS compiles with `dtc` with no warnings beyond the stock file's.

- **SDMMC4 (`mmc@700b0600`):** `status = "okay"`, `bus-width = <8>`, `non-removable`, exactly as mainline enables the Jetson TX1 module's eMMC (`tegra210-p2180.dtsi`). Supplies come from NVIDIA's power tree for the P3448 module (`tegra210-porg-power-tree-p3448-0000-a00.dtsi`): vqmmc is the MAX77620's SD3 (`max77620_sd3`, 1.8 V), which mainline's DTS labels `vdd_1v8`, and vmmc is `p3448_vdd_3v3_sys`, mainline's `vdd_3v3_sys`. Both rails already exist in the dev kit DTS.
- **Speed modes are not overridden.** `tegra210.dtsi`'s SDMMC4 node already declares `mmc-hs400-1_8v` with its `nvidia,dqs-trim` and pad calibration, which is what the TX1 runs on mainline. NVIDIA's DTS for this module also runs HS400, and adds `mmc-hs400-enhanced-strobe`. No mainline Tegra210 DTS enables enhanced strobe, so the patch leaves it out. CBoot's "ddr50 mode" is the bootloader's own choice and doesn't bind the kernel. If the bench shows CRC or tuning errors, capping the speed (e.g. `no-mmc-hs400`) is the first thing to try.
- **SDMMC1 (`mmc@700b0000`) disabled.** It's the P3448-0000 module's microSD slot; NVIDIA's `tegra210-p3448-0002-p3449-0000-b00.dts` disables it too.
- **SDMMC3 (`mmc@700b0400`) disabled as well.** Mainline enabled it for SDIO cards in the P3449 carrier's M.2 Key E slot (commit 1f32a31fe27e). NVIDIA's DTS leaves it disabled on this module (`tegra210-porg-p3448-common.dtsi`), a Turing Pi 2 node has no such slot, and GoSD's kernel builds no WLAN drivers.
- **`mmc0 = "/mmc@700b0600"` added to the existing `aliases`** (ethernet, rtc0, rtc1, serial0 are kept). The host index follows the alias, and `mmcblk` disk names follow the host index (`drivers/mmc/core/block.c`), so `APP` is `/dev/mmcblk0p1`, gosd-init's first boot candidate.
- **Data candidates:** `/dev/mmcblk0p2` is NVIDIA's `DXB` partition, which holds a raw DTB (`flash_l4t_t210_emmc_p3448.xml`); `mmcblk1` no longer exists. Read from the code: `dataexpand` refuses NVIDIA's protective MBR in `checkGosdMBR` before it writes anything, and `MountDataPartition` only calls `mount(2)`, which fails without writing. The failure isn't `ENOENT`, though, so gosd-init retries for `dataMountTimeout` (10 s) on every boot before falling back to a read-only `/data`. That's for bean gosd-j18s to remove.
- **Kernel config:** `CONFIG_EFI_PARTITION` (the kernel must parse NVIDIA's GPT for `APP` to exist), `CONFIG_NVME_CORE` and `CONFIG_BLK_DEV_NVME` (for gosd-j18s) are now in the fragment and `RequiredY`. The arm64 defconfig already builds all three in.

Sources:
- Linux v6.18.60: `arch/arm64/boot/dts/nvidia/tegra210.dtsi`, `tegra210-p2180.dtsi` and `tegra210-p3450-0000.dts`.
- NVIDIA L4T R32.7.6's device trees as mirrored by OE4T: https://github.com/OE4T/linux-tegra-4.9/tree/oe4t-patches-l4t-r32.7.6/nvidia/platform/t210/porg/kernel-dts (`tegra210-p3448-0002-p3449-0000-b00.dts`, `tegra210-porg-p3448-common.dtsi`, `porg-platforms/tegra210-porg-power-tree-p3448-0000-a00.dtsi`, `porg-platforms/tegra210-porg-pmic-p3448-0000-a00.dtsi`).
