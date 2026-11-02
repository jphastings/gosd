---
# gosd-cxio
title: 'Jetson Nano: board profile + mainline Tegra210 kernel (internal-only)'
status: completed
type: task
priority: normal
created_at: 2026-11-01T11:18:51Z
updated_at: 2026-11-01T12:16:58Z
parent: gosd-72je
---

Register `jetson-nano` as an internal-only board (`RegisterInternal`) with its kernel build, in one
PR. That's the turing-rk1 precedent (commit c725551): `internal/repocheck` requires a registered
board to have a `kernelspec` entry and a `build/boards/<id>/` directory in the same change. Locked
decisions live in the parent epic; follow them.

- [x] `internal/boards/jetsonnano`: `BootFiles` (Image, DTB, initramfs, extlinux.conf with
      `console=ttyS0,115200n8`), `RawWrites` empty, `FirmwareFiles` = `nvidia/tegra210/xusb.bin`
      (pinned linux-firmware URL + sha256), `Arch()` arm64, and the console-baud, EXT4 and
      USB-gadget answers backed by evidence at the pinned kernel tag
- [x] `internal/kernelspec` entry on `fleetKernelTag` (arm64 `defconfig` + a trimmed Tegra210
      fragment, `RequiredY` including `CONFIG_TEGRA_MC`), plus `build/boards/jetson-nano/`
- [x] CI kernel job in build-artifacts.yml (not in the release upload list: that's the activation
      bean)
- [x] CLAUDE.md Board IDs / Target lists, and `docs/development/jetson-nano.md` seeded with the
      epic's bench-relevant findings
- [x] A real `gosd build-kernel --board jetson-nano` run, with its kernel.config snapshot committed
- [x] A bench image built with `--artifacts-dir` from that run

## Findings

Sources: NVIDIA's L4T U-Boot as imported by OE4T, `l4t/l4t-r32.6.1-v2020.04` @ 46e4604c (U-Boot
paths below are in that tree), and linux `v6.18.37` (the fleet tag).

1. **2GB kit: ship the 4GB DTB only, via `fdt`.**
   - NVIDIA's side of the chain is proven. The flash tooling picks the `p3541-0000` U-Boot build
     for module SKU 0003, and `p3450-0000` otherwise (`p3448-0000.conf.common`:58-66,
     ackPeng/r32.7.6 @ e32c0213).
   - No `fdtfile` is set anywhere. `TEGRA` implies `DISTRO_DEFAULTS`, which selects
     `ENV_VARS_UBOOT_CONFIG`, so the default environment carries `board=<SYS_BOARD>` and
     `soc=tegra210` (`include/env_default.h`:91-105; `board/nvidia/p3541-0000/Kconfig`).
   - `fdtdir` therefore resolves to `tegra210-p3541-0000.dtb` (`cmd/pxe_utils.c`:564-605).
   - The broken link is the mainline DTS: there's no `tegra210-p3541-0000.dts` at v6.18.37. It
     lands in v6.19 (torvalds commit ba97758a63b7). Picking this up is a follow-up for after the
     fleet tag bump.
   - `fdt` is also the safer key. A missing `fdtdir` blob falls back to CBoot's L4T DTB at
     `fdt_addr` (`pxe_utils.c`:633-634).
2. **Load-address budget: TIGHT for the initramfs.**
   - Addresses (`include/configs/tegra210-common.h`:53-60): `fdt_addr_r=0x83000000` (1 MiB before
     CBoot's DTB at 0x83100000), `ramdisk_addr_r=0x83200000`, `kernel_addr_r=0x84000000`,
     `scriptaddr=0x90000000`, `pxefile_addr_r=0x90100000`.
   - That leaves the initramfs at most **14 MiB** (14,680,064 B), and the kernel up to 192 MiB.
   - The pxe loader loads the initrd before the kernel (`pxe_utils.c`:465-478), so an oversize
     initramfs gets silently truncated by the kernel load.
   - Measured: a jetson-nano `examples/hello` initramfs is 10,936,458 B. `sattrack` is
     12,013,016 B and `usbwebsite` 12,255,043 B (both measured on qemu-virt). `hello` with
     `--ingress cloudflared` is 28,371,463 B, which does not fit.
   - The kernel is fine. The fleet `Image` is 67,066,368 B with `image_size` 0x40C0000, and
     `booti` doesn't move it (flags bit 3; `arch/arm/lib/image.c`).
   - The preboot-script hook can't help: `CONFIG_USE_PREBOOT` is off.
   - OE4T's own builds use a different layout (14b16137): kernel at 0x83200000, ramdisk at
     0x87200000. There the kernel is capped at 64 MiB and our `image_size` overlaps the initrd by
     768 KiB, but that's not stock firmware.
3. **USB gadget: claimed, per JP's "already free" exception.**
   - The DTS enables `usb@700d0000` (xudc). Its padctl port `usb2-0` is `mode = "peripheral"`
     with `usb-role-switch` and a VBUS GPIO connector (`tegra210-p3450-0000.dts`:520-536,
     580-586).
   - The arm64 defconfig has `USB_TEGRA_XUDC=m` (promoted to `=y` with no modules), and
     `PHY_TEGRA_XUSB` selects `USB_CONN_GPIO`. The released rock-4se `kernel.config` (same
     defconfig and tag) shows `CONFIG_USB_TEGRA_XUDC=y` and `CONFIG_USB_CONN_GPIO=y`.
   - Nothing is gadget-specific in the fragment or in RequiredY.
4. **EXT4 and console baud: both supported.** `CONFIG_EXT4_FS=y` comes from the arm64 defconfig
   (defconfig:1751; `=y` in the rock-4se snapshot), and the fragment doesn't cut it. The
   extlinux `append` renders `console=ttyS0,{{.ConsoleBaud}}n8`.
5. **Status LED: nothing to register.** gosd-init discovers the LED through sysfs, with no
   per-board table (`cmd/gosd-init/internal/statusled`). The p3450 DTS and `tegra210.dtsi` have
   no `gpio-leds` node, so there's nothing for a retain-state patch to target, and the LED is a
   silent no-op.
6. **Header buses: GPIO only.** The Tegra gpiochip is always on, and the fragment builds
   `GPIO_TEGRA` and `GPIO_CDEV`. The header I2C (`i2c@7000c000`, `i2c@7000c400`) and SPI
   (`spi@7000d400`, `spi@7000d600`) are `disabled` in `tegra210.dtsi` and the p3450 DTS doesn't
   enable them. Enabling them is a follow-up DTS patch, recorded in the dev notes.

Two corrections to the epic's text, for JP to accept:

- **R32.5.x isn't enough; R32.6.1 is the minimum.** R32.5.x copies `/chosen/bootargs` from
  CBoot's DTB over our `APPEND` (that branch's `fdt_copy_prop_paths`). NVIDIA fixed it in
  R32.6.1 (bug 3323547, commit e9ee05aa).
- **`tegra210-p3541-0000.dtb` isn't in mainline at the fleet tag.** It lands in v6.19.

Also:

- **Ethernet and SD depend on the PMIC.** The MAX77620 supplies the SD card's vqmmc and the
  PCIe rail. Its I2C/MFD/regulator/GPIO/pinctrl drivers are in RequiredY.
- **QSPI controller cut.** `SPI_TEGRA210_QUAD` is unset so the boot flash isn't exposed as
  `/dev/mtd0`.

## Summary of Changes

- **Board.** `jetson-nano` is registered internal-only (`RegisterInternal`) with its kernelspec
  entry, `build/boards/jetson-nano/` and a CI kernel job, all landing in one change as repocheck
  requires (the turing-rk1 shape).
- **No bootloader.** GoSD ships none for this board: NVIDIA's QSPI firmware runs distro boot on our
  standard MBR/FAT32 card, so `RawWrites` is empty and no NVIDIA blob is fetched or shipped.
- **Firmware.** The only third-party file is linux-firmware's `nvidia/tegra210/xusb.bin`, pinned to
  tag 20261019 by URL + sha256.
- **Kernel.** It's on the fleet tag (v6.18.37), with `CONFIG_TEGRA_MC` and the MAX77620 PMIC stack
  asserted. The QSPI flash driver is cut, so apps never see a writable `/dev/mtd0` over NVIDIA's
  bootloader. Every `RequiredY` symbol was verified `=y`, and the cuts verified unset, in the
  committed kernel.config from a real build-kernel run.
- **Bench image.** `examples/hello` built for jetson-nano from that run's artifacts. Read back
  from the image: initramfs 10,936,460 B, under the 14 MiB ceiling; extlinux.conf as rendered.
- **Not hardware-verified.** That's bean gosd-9o0m. The epic's QSPI minimum was corrected from
  R32.5 to R32.6.1 here, and the 14 MiB initramfs ceiling is bean gosd-0yrk.
