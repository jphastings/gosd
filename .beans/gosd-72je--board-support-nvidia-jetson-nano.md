---
# gosd-72je
title: 'Board support: NVIDIA Jetson Nano'
status: in-progress
type: epic
priority: normal
created_at: 2026-11-01T11:18:35Z
updated_at: 2026-11-07T20:59:57Z
---

Board support for the original NVIDIA Jetson Nano: Tegra X1 (T210), 4x Cortex-A57, arm64. **The target hardware is the 16GB eMMC production module, P3448-0002** (NVIDIA part 900-13448-0020-000). The reference carrier is a Turing Pi 2 node. It has Ethernet only (RTL8111 over PCIe) and no onboard WiFi.

## Locked decisions

- **eMMC module only (decided 2026-11-07, JP).** This replaces the original target, the SD-card developer kit (P3448-0000 on a P3450 carrier), which is dropped together with the 2GB kit. Its QSPI-firmware prerequisite (R32.6.1+) and SD-card layout no longer apply. The board ID stays `jetson-nano`, build tag `gosd_jetson_nano`.
- **NVIDIA's layout owns the eMMC.** Each flash writes NVIDIA's whole boot chain to eMMC `boot0`/`boot1`: BCT, nvtboot, CBoot, then the L4T U-Boot v2020.04 in `LNX`, the same `p3450-0000` build the dev kit uses. It also writes NVIDIA's 17-partition GPT to the user area. **GoSD's FAT32 boot partition is written into `APP`**, partition 1 (14 GiB, starting at sector 0x22). NVIDIA's U-Boot runs distro boot: no SD card, so it moves to `mmc0`, falls back to partition 1, and reads `/extlinux/extlinux.conf`. This was proven on the bench, see gosd-9o0m. GoSD writes no other partition.
- **`/data` lives on the NVMe drive (decided 2026-11-07, JP),** never on the eMMC. The design is bean gosd-j18s.
- **Flashing is NVIDIA's massflash, driven from the Turing Pi 2 BMC** (`docs/jetson-nano-flashing.md`):
  1. NVIDIA's `nvmassflashgen.sh` signs a portable blob offline on any Linux host.
  2. The BMC runs NVIDIA's own `nvmflash.sh`, with its 32-bit x86 tools under `qemu-i386`, against the recovery USB device on its native USB port.

  Doing all of this in Go is bean gosd-32dw. **Flashing through a Mac's USB passthrough into a VM is not viable**, whatever the VM: about 30 attempts across five configurations produced one complete flash, and the module's own log blamed data lost on the USB link (`Data recieve from host failure with error 0x120000`). `tpi flash` is never used: it raw-writes a disk image over the eMMC and doesn't speak NVIDIA's protocol.
- **GoSD never fetches or re-hosts NVIDIA blobs itself today.** The flashing procedure downloads NVIDIA's BSP from NVIDIA. Whoever later automates it (gosd-32dw) does so at a pinned URL and sha256, per the project-wide third-party-blob decision.
- **Mainline-only, on the mainline fleet kernel tag** (`fleetKernelTag`). We always ship the mainline `tegra210-p3450-0000.dtb` via `FDT`, and never boot a mainline kernel on the L4T DTB (OpenWrt saw timer, GIC and PMC breakage). `CONFIG_TEGRA_MC` is required: Armbian found the PMC probe defers forever without it. Enabling the eMMC controller in that DTB is bean gosd-wxy7.
- **Serial console `ttyS0,115200n8` (UART-A), confirmed.** On a Turing Pi 2 it comes out only on the node's "UART2 DEBUG" header, not on the BMC's node console.
- **Firmware:** `nvidia/tegra210/xusb.bin` from linux-firmware, at a pinned URL and sha256, through `FirmwareFiles`. It was confirmed loading on the bench. `vic04_ucode.bin` (the HDMI framebuffer) is not shipped.
- **The initramfs has a 14 MiB ceiling.** The eMMC module runs the same U-Boot build and default environment: `ramdisk_addr_r` 0x83200000, `kernel_addr_r` 0x84000000. `gosd build` refuses anything bigger (gosd-0yrk). `hello` (10.9 MB) loaded fine on the bench.

## Known couplings

- **NVIDIA's U-Boot edits our DTB before booting.** It copies `/memory`, the MACs, `/serial-number` and the carveouts from CBoot's DTB. It skips `/external-memory-controller@7001b000` with a benign "phandle mismatch" (seen on the bench), and prints "Can't set DT prop /bpmp/carveout-start: FDT_ERR_BADOFFSET", which is also benign so far. The source is `fdt_copy_node_paths` / `arch/arm/mach-tegra/dt-edit.c` and `cboot.c` in https://github.com/OE4T/u-boot-tegra/tree/l4t/l4t-r32.6.1-v2020.04.
- **The EMC (RAM) clock stays at its boot rate** under mainline. That's accepted.

## Out of scope

- **The SD-card dev kit (P3448-0000) and the 2GB kit (P3541).** Dropped 2026-11-07.
- **The Jetson Orin Nano (T234).** It's a separate epic if ever wanted.
- **Mainline U-Boot.** NVIDIA's U-Boot already boots our files.

## Lifecycle

NVIDIA sells the Jetson Nano module until January 2027. The final T210 software release is R32.7.6 / JetPack 4.6.6 (Nov 2024), so the firmware we flash is a frozen target.

## Sources

- Boot flow: https://docs.nvidia.com/jetson/archives/l4t-archived/l4t-3251/Tegra%20Linux%20Driver%20Package%20Development%20Guide/bootflow_jetson_nano.html
- U-Boot extlinux keys: https://docs.nvidia.com/jetson/archives/l4t-archived/l4t-3251/Tegra%20Linux%20Driver%20Package%20Development%20Guide/uboot_guide.html
- Armbian, 6.18 with `TEGRA_MC` required: https://github.com/armbian/build/pull/11034
- Firmware needs: https://github.com/armbian/firmware/pull/148
- The L4T DTB breaks mainline: https://forums.developer.nvidia.com/t/support-nano-on-openwrt/219168/7
- eMMC module layout: https://github.com/ackPeng/r32.7.6/blob/HEAD/bootloader/t210ref/cfg/flash_l4t_t210_emmc_p3448.xml
- Offline massflash: `README_Massflash.txt` in the L4T R32.7.6 BSP
- Lifecycle: https://developer.nvidia.com/embedded/lifecycle
