# Developing for the NVIDIA Jetson Nano (`jetson-nano`)

Bench and bring-up knowledge for the original Jetson Nano SD-card dev kit
(Tegra210; P3450 carrier, P3448-0000 module). The locked decisions live in
epic `gosd-72je` and CLAUDE.md; this file is for what a future bring-up
would otherwise have to rediscover. Everything below was established from
source before any hardware was on the bench, so treat each claim as a
hypothesis until bench bean `gosd-9o0m` confirms it.

The U-Boot facts come from NVIDIA's L4T U-Boot source as imported by OE4T,
[branch `l4t/l4t-r32.6.1-v2020.04`](https://github.com/OE4T/u-boot-tegra/tree/46e4604c78d3804ccd4cf9624460a86ea5318a61).
It's the newest NVIDIA import there; no R32.7.x source was found, so the
final R32.7.6 firmware is assumed unchanged until the bench's `printenv`
says otherwise.

## The QSPI firmware must be R32.6.1 (JetPack 4.6) or newer

Not merely R32.5, as the epic first stated. In R32.5.x, U-Boot copies
`/chosen/bootargs` out of CBoot's DTB into the DTB we supply after writing
our `APPEND` line into it, so our kernel command line is silently replaced
(`fdt_copy_prop_paths` in `include/configs/tegra210-common.h` on
[the R32.5 branch](https://github.com/OE4T/u-boot-tegra/blob/l4t/l4t-r32.5-v2020.04/include/configs/tegra210-common.h)).
NVIDIA fixed it as bug 3323547
([commit e9ee05aa](https://github.com/OE4T/u-boot-tegra/commit/e9ee05aa8e07b7da360a63a24ed85f1b6a40b8bb)),
and R32.6.1 no longer copies it. The recovery the epic already prescribes,
booting NVIDIA's JetPack 4.6 SD image once, lands on R32.6.1, so it still
works. User-facing docs should say JetPack 4.6 or newer.

## Serial console

UART-A as `ttyS0`, 115200n8, 3.3 V. On the B01 carrier it's J50 pins 4
(TX), 3 (RX), and 7 (GND); on the A02 carrier it's J44. The DTS aliases
`serial0` to UART-A, and `8250_tegra` numbers its port by that alias
(`uart_read_port_properties`), so `ttyS0` is expected. Confirm it on the
first boot anyway: turing-rk1's alias-derived device name was wrong on
real hardware.

## Load-address budget: the initramfs is the tight one

The stock R32.6.1 U-Boot loads at fixed addresses (`tegra210-common.h`
lines 53-60). Tegra210 doesn't use the `calculated_vars` mechanism the
later SoCs do.

| Variable | Address | Room before the next thing |
| --- | --- | --- |
| `fdt_addr_r` | `0x83000000` | 1 MiB (CBoot's own DTB sits at `0x83100000`) |
| `ramdisk_addr_r` | `0x83200000` | **14 MiB** (14,680,064 B) up to `kernel_addr_r` |
| `kernel_addr_r` | `0x84000000` | 192 MiB up to `scriptaddr` |
| `scriptaddr` | `0x90000000` | |
| `pxefile_addr_r` | `0x90100000` | |
| `fdtoverlay_addr_r` | `0x90200000` | |

The kernel isn't the problem. The fleet's arm64 `Image` is about 64 MiB,
and its header `image_size` (BSS included) is 64.75 MiB. That fits easily,
and `booti` leaves it where it was loaded, because the header's flags allow
any 2 MiB-aligned base.

The initramfs is the problem. `label_boot` in `cmd/pxe_utils.c` loads the
initrd first and the kernel second, so an initramfs over 14 MiB has its
tail silently overwritten by the kernel load. On serial that shows up as
"Initramfs unpacking failed", followed by a `panic=10` reboot loop.

`gosd build` now refuses a jetson-nano image whose initramfs exceeds this
ceiling, at build time, rather than leaving the failure to the device.

Measured initramfs sizes:

- `examples/hello` built for this board: 10.4 MiB.
- `sattrack` and `usbwebsite`, measured on qemu-virt: 11.5-11.7 MiB.
- Any `--ingress cloudflared` build: about 27 MiB, which won't boot.

Nothing in `gosd build` guards this limit yet.

Two other ways the layout could differ from the table above:

- Meta-tegra (Yocto) builds of this U-Boot move `kernel_addr_r` to
  `0x83200000` and `ramdisk_addr_r` to `0x87200000`
  ([OE4T commit 14b16137](https://github.com/OE4T/u-boot-tegra/commit/14b16137af48979270d096efbd209b74b03d2627)).
  That caps the kernel at 64 MiB, and our `image_size` overlaps the initrd
  by 768 KiB. This only matters if the QSPI was flashed from a Yocto build
  rather than by NVIDIA's tools.
- The environment is stored in the QSPI (and in eMMC on the production
  module). An old `saveenv` overrides every default above.

So the first bench step is `printenv kernel_addr_r ramdisk_addr_r
fdt_addr_r board soc fdtfile` at the U-Boot prompt
(`Tegra210 (P3450-0000) #`).

The `u-boot-preboot.scr` hook in `p3450-0000.h` can't lift the limit. It
looks like it could, but `CONFIG_USE_PREBOOT` is off in
`p3450-0000_defconfig`, so `main_loop` never runs the hook.

## What NVIDIA's U-Boot does to our DTB

Before booting, `ft_system_setup` (`arch/arm/mach-tegra/dt-setup.c`)
edits the DTB we supply:

- **GPU:** it enables the GM20B node.
- **Copied nodes:** it copies these nodes from CBoot's DTB:
  - `/memory@80000000`, which gives the real RAM size (useful).
  - `/external-memory-controller@7001b000`. Expect a "phandle mismatch"
    skip here; it's benign.
  - Several `/chosen/*-board` nodes.
- **Copied properties:** it copies the BPMP carveout, the MACs,
  `/serial-number`, and `linux,initrd-start` and `linux,initrd-end`.

The initrd values are rewritten afterwards by U-Boot's own `fdt_initrd`
(`common/image-fdt.c` runs it after `ft_system_setup`), so ours win.

The Ethernet MAC is written into the RTL8111 node through the DTB's
`ethernet` alias (`ft_mac_address_setup` in
`board/nvidia/p3450-0000/p3450-0000.c`), and the mainline DTS defines that
alias. A MAC that changes on every boot means that chain broke.

`extlinux.conf` names its DTB with `fdt`, never `fdtdir`, for a reason. If
an `fdt` file is missing, U-Boot skips the label. If an `fdtdir` lookup
misses, it falls back to CBoot's L4T DTB at `fdt_addr`, and the epic
forbids that pairing.

## Things to watch on the first boots

- **FAT boot.** No report of a Nano booting from FAT was found, so this is
  the first thing to check.
- **USB.** If `tegra-xusb` says "failed to request firmware", USB will be
  missing: `xusb.bin` has to be in the initramfs under
  `nvidia/tegra210/`.
- **SD card or Ethernet doesn't probe.** Check the MAX77620 PMIC first
  (i2c@7000d000, address 0x3c). It supplies the SD card's I/O rail and the
  PCIe rail the RTL8111 sits behind.
- **RAM speed.** The fleet config leaves `CONFIG_TEGRA210_EMC` unset, so
  RAM stays at its boot clock. This is expected.
- **QSPI is hidden on purpose.** The QSPI controller driver is cut, so no
  `/dev/mtd*` exposes the boot flash to app code.
- **Status LED.** The dev kit has none that software can drive: there's no
  `gpio-leds` node, and the power LED is hard-wired. The status LED is
  therefore a silent no-op, as on qemu-virt.

## Follow-ups not done in the board-profile bean

- **The 2GB kit (P3541).** NVIDIA's side is proven:
  - The flash tooling puts the `p3541-0000` U-Boot build on module SKU 0003
    (`p3448-0000.conf.common` lines 58-66 in
    [an R32.7.6 BSP mirror](https://github.com/ackPeng/r32.7.6/blob/e32c0213e16de6bac4d5e33fa97dd8b193d1155b/p3448-0000.conf.common)).
  - That build's default environment has `board=p3541-0000` and
    `soc=tegra210`, and no `fdtfile`.
  - So `fdtdir` would resolve to `tegra210-p3541-0000.dtb` by itself
    (`cmd/pxe_utils.c` lines 564-605).

  What's missing is the mainline DTS. It only lands in v6.19
  ([commit ba97758a63b7](https://github.com/torvalds/linux/commit/ba97758a63b78f7e494ebdfb74ba8a8af702a33a)),
  after the fleet's v6.18 tag. Once the fleet moves to v6.19:
  - ship both DTBs (an `AdditionalDTBs` entry);
  - switch the template to `fdtdir /`.
- **Header I2C and SPI.** The J41 header's I2C buses (`i2c@7000c000`,
  `i2c@7000c400`) and SPI buses (`spi@7000d400`, `spi@7000d600`) are
  disabled in `tegra210.dtsi`, and the p3450 DTS doesn't enable them. They
  need a kernel-build DTS patch. The drivers are already built: I2C_TEGRA
  is required for the PMIC anyway, and SPI_TEGRA114 comes from defconfig.
  Header GPIO works now through the Tegra gpiochip.
- **Lifting the 14 MiB initramfs ceiling** (see the budget section above).
- **USB gadget.** It's claimed from the DTS and the fleet's config, but it
  hasn't been verified on hardware.
