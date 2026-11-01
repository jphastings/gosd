---
# gosd-72je
title: 'Board support: NVIDIA Jetson Nano'
status: in-progress
type: epic
created_at: 2026-11-01T11:18:35Z
updated_at: 2026-11-01T11:18:35Z
---

Board support for the original NVIDIA Jetson Nano: Tegra X1 (T210), 4x Cortex-A57, arm64. Target
hardware is the **SD-card developer kit**: the P3450 carrier with the P3448-0000 module (4GB, B01/A02
revisions). Its microSD slot is on the module itself. Ethernet only (RTL8111 GbE over PCIe), no
onboard WiFi.

## Locked decisions

- **Board ID: `jetson-nano`** (build tag `gosd_jetson_nano`). It covers the 4GB dev kit. The 2GB kit
  (P3541) is deferred until the fleet kernel tag reaches v6.19, because `tegra210-p3541-0000.dts`
  only lands upstream there. NVIDIA's side is already proven (bean gosd-cxio's findings): the
  p3541 U-Boot build's env has `board=p3541-0000`, so extlinux `fdtdir` would pick its DTB by
  itself. Until then the 4GB DTB ships via `fdt`. That's also the safer key: a missing `fdtdir`
  blob falls back to CBoot's L4T DTB, the pairing forbidden below.
- **GoSD ships no bootloader for this board and never fetches or re-hosts any NVIDIA blob.** Since
  L4T R32.5 (JetPack 4.5), the dev-kit module keeps its whole boot chain in its own QSPI-NOR:
  BootROM → nvtboot → nvtboot-cpu → CBoot → U-Boot (L4T fork, v2020.04, in the `LNX` partition)
  → kernel. The final release for T210 is R32.7.6 / JetPack 4.6.6 (Nov 2024), so that firmware is a
  frozen target. `RawWrites` is empty.
- **The card stays GoSD's standard layout** (MBR, FAT32 boot partition 1, data partition 2), with
  no change to the on-card layout ABI. NVIDIA's U-Boot runs ordinary distro boot. It tries
  `boot_targets` mmc1 (SD) first. It falls back to partition 1 when nothing is flagged bootable,
  and it doesn't look for an `APP` label. `DOS_PARTITION`, `CMD_FAT` and `CMD_EXT4` are compiled
  in, and it reads `/extlinux/extlinux.conf` and honours `FDT`/`INITRD`/`APPEND`. `${cbootargs}`
  is only used if `APPEND` references it, and ours won't.
- **Mainline-only, on the mainline fleet kernel tag** (`fleetKernelTag`), with the mainline
  `tegra210-p3450-0000.dtb` always supplied via `FDT`. Never boot a mainline kernel on the L4T
  DTB in QSPI: OpenWrt tried and got timer/GIC/PMC breakage. `CONFIG_TEGRA_MC` is required;
  Armbian found the PMC probe defers forever without it.
- **Serial console `ttyS0,115200n8`** (UART-A). Pins are J50 4/3/7 (TX/RX/GND) on B01, J44 on A02.
- **Firmware:** `nvidia/tegra210/xusb.bin` from linux-firmware, at a pinned URL + sha256, through
  `FirmwareFiles`. Every USB port sits behind XUSB, so without it there's no USB host (and no
  `disk` package over USB). `vic04_ucode.bin` (HDMI framebuffer) is NOT shipped: there's no display
  use case.
- **End-user prerequisite: QSPI on R32.6.1 (JetPack 4.6) or newer** (corrected from R32.5 by bean
  gosd-cxio). A unit whose QSPI predates R32.5 won't boot our card at all, with no error. On
  R32.5.x, U-Boot copies CBoot's `/chosen/bootargs` over our `APPEND`, which silently replaces our
  kernel command line. That's NVIDIA bug 3323547, fixed in R32.6.1 by u-boot commit e9ee05aa. The
  fix needs only an SD card: boot NVIDIA's JetPack 4.6 SD image once, and it updates the QSPI
  itself. User-facing docs must say this up front.
- **The initramfs has a 14 MiB ceiling on stock firmware** (bean gosd-cxio's findings). NVIDIA's
  U-Boot env puts `ramdisk_addr_r` at 0x83200000 and `kernel_addr_r` at 0x84000000. sysboot loads
  the initrd first, so a bigger one has its tail overwritten by the kernel and fails at unpack,
  with no build-time warning. `hello` fits (~10.9 MB); anything with `--ingress cloudflared`
  doesn't. Refusing or lifting it is a separate bean that blocks activation.

## Known couplings and risks (for the bench to settle)

- **NVIDIA's U-Boot edits our DTB before booting.** It copies these from CBoot's DTB:
  `/memory@80000000` (useful: the real RAM size), `/external-memory-controller@7001b000` (skipped
  with "phandle mismatch" when phandles differ), several `/chosen` nodes, MAC addresses,
  `/serial-number`, the bpmp/framebuffer carveouts, and it enables the GM20B node. The source is
  `fdt_copy_node_paths` / `arch/arm/mach-tegra/dt-edit.c`, `cboot.c` in
  https://github.com/OE4T/u-boot-tegra/tree/l4t/l4t-r32.6.1-v2020.04 (same list at R32.7.6).
- **The EMC (RAM) clock probably stays at its boot rate** under mainline. Mainline's EMC scaling
  needs a firmware-trained table that NVIDIA's firmware doesn't provide in that format. Accepted:
  it doesn't matter for a GoSD app.
- **No report of a Nano booting from FAT was found.** FAT is compiled in, but Armbian booted ext4.
  This is the first thing the bench checks.

## Out of scope

- **The eMMC production module (P3448-0002), including Turing Pi 2 Nano nodes.** It has no QSPI.
  Its boot chain is in eMMC boot0 plus an NVIDIA GPT in the user area, so a GoSD MBR image can't
  be written over it. Flashing needs SDK Manager / `flash.sh` on x86 Ubuntu; the Turing Pi BMC only
  toggles recovery mode.
- **The Jetson Orin Nano (T234).** Its UEFI L4TLauncher accepts GPT only, with a partition named
  `APP`, which conflicts with the MBR layout ABI. It's a separate epic if ever wanted.
- **Mainline U-Boot in QSPI.** Rewriting `LNX` needs NVIDIA's x86 `flash.sh`, and it adds little
  over the stock U-Boot.

## Lifecycle

NVIDIA sells the Jetson Nano module until January 2027; second-hand dev kits are plentiful.

## Sources

- Boot flow: https://docs.nvidia.com/jetson/archives/l4t-archived/l4t-3251/Tegra%20Linux%20Driver%20Package%20Development%20Guide/bootflow_jetson_nano.html
- Everything moved to QSPI in R32.5: https://forums.developer.nvidia.com/t/what-version-of-jetpack-has-the-feature-kernel-boot-sequence-only-jetpack4-5-how-can-we-make-a-usb-disk-image-with-an-easy-way/182864?page=3
- U-Boot extlinux keys: https://docs.nvidia.com/jetson/archives/l4t-archived/l4t-3251/Tegra%20Linux%20Driver%20Package%20Development%20Guide/uboot_guide.html
- Armbian, mainline 5.10 on stock QSPI via extlinux: https://github.com/armbian/build/pull/2720
- Armbian, 6.18 on stock QSPI, `TEGRA_MC` required: https://github.com/armbian/build/pull/11034
- Firmware needs: https://github.com/armbian/firmware/pull/148
- L4T DTB breaks mainline: https://forums.developer.nvidia.com/t/support-nano-on-openwrt/219168/7
- Updating old QSPI from an SD card: https://forums.developer.nvidia.com/t/jetpack-4-5-production-release-with-l4t-32-5/166475
- eMMC module layout: https://github.com/ackPeng/r32.7.6/blob/HEAD/bootloader/t210ref/cfg/flash_l4t_t210_emmc_p3448.xml
- Lifecycle: https://developer.nvidia.com/embedded/lifecycle
