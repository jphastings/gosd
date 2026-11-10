# Developing for the NVIDIA Jetson Nano (`jetson-nano`)

Bench and bring-up knowledge for the Jetson Nano 16GB eMMC production
module (Tegra210, P3448-0002), the only Nano GoSD targets since 2026-11-07.
The SD-card developer kit (P3448-0000) was dropped, together with its
QSPI-firmware prerequisite. The locked decisions live in epic `gosd-72je`
and CLAUDE.md; this file is for what a future bring-up would otherwise have
to rediscover.

The bench is a v2.4 Turing Pi 2, node 4, BMC firmware 2.3.4. The module
reports `pm board id is 3448-0002-401`, revision G.0. How it gets flashed
is in [the Jetson Nano flashing guide](../jetson-nano-flashing.md), and
the Turing Pi side is in [the shared Turing Pi 2 BMC notes](turing-pi-2-bmc.md).

## What the bench proved (2026-11-07, bean `gosd-9o0m`)

- **A blank module does nothing visible.** Straight from the distributor,
  it prints nothing on its console, presents no recovery-mode USB device and
  sends no network traffic. The Ethernet link still comes up at 100 Mbps,
  but that's just the network chip's default, so it's no sign of life.
- **NVIDIA's U-Boot boots our FAT32 boot partition from `APP`.** The flash
  writes GoSD's boot partition into `APP`, partition 1 of NVIDIA's GPT
  (sectors 0x22 to 0x1c00021, 14 GiB). The U-Boot is the R32.7.6 build,
  `U-Boot 2020.04-g904285e3a7 (Nov 04 2024)`:
  - It runs distro boot: `boot_targets=mmc1 mmc0 usb0 nvme0 pxe dhcp`, with
    `bootdelay=2`.
  - It reports `bad CRC, using default environment`, so no saved
    environment overrides anything.
  - It prints `Scanning mmc 0:1... Found /extlinux/extlinux.conf`, then
    loads our initramfs, `Image` and DTB.

  So the open question of whether a Nano boots from FAT is settled: it does.
- **The mainline kernel comes up.** With our command line, 6.18.37 brings
  up:
  - `r8169` (RTL8168f/8111f), with the MAC that CBoot puts in
    `/chosen/nvidia,ethernet-mac` and U-Boot copies into our DTB. CBoot
    logs the EEPROM's own MAC fields as blank (`all 0xff`) on this module,
    so where it gets the address from is NVIDIA's business;
  - `tegra-xusb`, with our pinned `xusb.bin` loaded;
  - the SDMMC1 and SDMMC3 controllers.
- **gosd-init can't find its boot partition yet.** It tries
  `/dev/mmcblk0p1`, `/dev/mmcblk1p1` and `/dev/vda1` for 10 s, logs
  `fatal: mounting boot partition failed`, and reboots about every 20 s.
  The mainline `tegra210-p3450-0000` DTS leaves SDMMC4 (`mmc@700b0600`,
  the eMMC) disabled, because the dev kit's module has none. Bean
  `gosd-wxy7` adds a kernel-build DTS patch that enables SDMMC4, disables
  SDMMC1 and SDMMC3, and aliases the eMMC to `mmc0`, so `APP` should
  become `/dev/mmcblk0p1`. With that patch the eMMC comes up as an HS400
  `mmcblk0`, and gosd-init mounts `APP` as its boot partition.
- **U-Boot sees the M.2 NVMe drive.** That's `nvme0`, the 477 GB drive in
  node 4's slot, where `/data` lives (see below). U-Boot also gets a DHCP
  lease, so PCIe and Ethernet work before Linux does.
- **The status LED is a silent no-op,** as expected: there's no `gpio-leds`
  node.
- **The initramfs fits.** `hello`'s 10.9 MB initramfs loads within the
  14 MiB ceiling below.

## Where `/data` lives: the NVMe drive

NVIDIA's GPT owns the eMMC: partition 2 is its raw `DTB`, and U-Boot itself
is in `LNX`. So the board declares `boards.NVMeDataBoard`, the build bakes
`dataOnNVMe` into `config.json` and ships no data partition, and gosd-init
never tries a partition-2 candidate or runs dataexpand. Instead it hands
the drive to `internal/blockmount`, the machinery behind the public `disk`
package, as a whole-device ext4 volume labelled `<prefix>-data` (bean
`gosd-j18s` has the design and the crash-ordering argument). Only names
matching `nvme<N>n<N>` are ever candidates, so nothing on the eMMC can be
chosen.

On the console, after `data partition filesystem: ext4`, expect one of:

| The drive | gosd-init says |
| --- | --- |
| blank | `formatting the NVMe drive at /dev/nvme0n1 as ext4, labelled <prefix>-data`, then `/data is a new ext4 volume on the NVMe drive at /dev/nvme0n1` |
| this app's volume | `/data is the ext4 volume labelled <prefix>-data already on the NVMe drive at /dev/nvme0n1` |
| anything else | `mounting data partition failed; mounting /data read-only: the NVMe drive at /dev/nvme0n1 already holds …; refusing to reformat it … — GoSD only formats a blank drive, …` |
| not fitted | after 3 s: `mounting data partition failed; mounting /data read-only: no NVMe drive found: the PCIe bus has had no NVMe controller on it for 3s; …` |
| no driver takes it | after 30 s: `… an NVMe controller is on the PCIe bus, but no driver took it within 30s; …` |
| no namespace | after 30 s: `… the NVMe driver took the controller, but no writable namespace on it appeared within 30s; …` |
| several namespaces | `found NVMe namespaces …; /data is the one labelled <prefix>-data, …` when exactly one carries it, otherwise `… so GoSD won't guess which is /data; …` and nothing is written |

Every success then logs `data partition mounted read-write at /data`.
"Blank" means the drive's first MiB reads as zeroes. U-Boot's `No partition
table - nvme 0` doesn't prove that, and it stays true after GoSD has
formatted the drive, since the volume has no partition table.

The first boot on a blank drive takes longer: it writes GoSD's 512 MiB ext4
seed image, superblock last, then grows it online to fill the drive. A
power cut part-way through leaves the first MiB still zero, so the next boot
simply formats again.

The bench (2026-11-08, node 4's 477 GB drive) showed each case in turn:
- a drive holding another ext4 volume was refused;
- a blank drive was formatted;
- the next boot adopted it;
- a reflash with a different image adopted it again and put back the
  `config/hostname` edit kept on `/data`.

To set that up without a keyboard on U-Boot's console, a throwaway app
cleared the old volume's first MiB. The hostname edit went into the image's
boot partition before signing, standing in for someone editing the card.

On 2026-11-10 a `v0.10.7` `hello` image, flashed by `jetson-flash` on node 4,
came up as `nanobench` rather than `hello`, with its boot counter already
at 1. That hostname edit and the counter had survived an erase of the whole
eMMC, so after a reflash look for the module under its old name or its DHCP
address: `hello.local` won't answer.

Hard power cuts on that image (`tpi power off`, 10 s, `tpi power on`): 11 of
12 came back, about 25 s to HTTP, with the counter going up by one each
time. The other was the first cut after the flash, straight after the
recovery strap was released and after 12 minutes of uptime. It hadn't
answered HTTP after 404 s, and the counter shows it never reached the app.
The serial adapter wasn't on node 4's header, so where it stopped is
unknown. Toggling the strap first (3 tries) and 12 minutes of uptime
(1 try) didn't bring it back.

## Things that look alarming but aren't

On every boot, CBoot logs EEPROM read failures for `1-a0` and `3-ae`, and
U-Boot logs `phandle mismatch` for `external-memory-controller@7001b000`
and `Can't set DT prop /bpmp/carveout-start: FDT_ERR_BADOFFSET`. The boot
carries on regardless. The EEPROM failures are the carrier-board EEPROMs a
Turing Pi doesn't have; the module's own EEPROM, `3-a0`, reads fine.

## How flashing was found to work (and not)

- **U-Boot's `ums` doesn't get through a Turing Pi 2.** NVIDIA's U-Boot
  includes `ums`, and `ums 0 mmc 0:1` starts and exposes exactly `APP`
  (`sector 0x22, count 0x1c00000`). But nothing enumerates, on the BMC or on
  a computer on the `USB_OTG` socket, whichever `tpi usb` routing is used.
  That's despite the boot ROM's recovery device coming through the same
  port. The best guess is that U-Boot's gadget waits for VBUS, which the
  slot doesn't present; that hasn't been proven. So the flash writes `APP`
  together with NVIDIA's boot chain instead.
- **Flashing from a VM on a Mac failed, every way we tried.** The
  configurations:
  - an x86 VM with QEMU's `usb-host` on emulated xHCI and on EHCI, with
    and without `pipeline=off`;
  - UTM's `usbredir` sharing, with x86 and with arm64 guests;
  - an arm64 VM at native speed under HVF.

  About 30 attempts produced one complete flash. The module's own log says
  why (`Data recieve from host failure with error 0x120000` mid-transfer):
  data doesn't arrive intact, even when the host side reports every byte
  sent. Slow hosts fail sooner, because the boot ROM applet and CBoot only
  wait briefly between steps. A gap of about 0.6 s between the BCT and the
  bootloader sometimes worked; about 3.7 s never did.
- **Gotchas when running NVIDIA's tools on arm64 Linux.** The tools are
  static 32-bit x86. Offline signing works under Ubuntu's stock
  `qemu-user-static`. Their USB calls only work with the emulated program
  mapped at address 0 (`QEMU_GUEST_BASE=0`), because qemu 6.2 passes
  `USBDEVFS_BULK` data pointers to the kernel untranslated (`EFAULT`
  otherwise). That in turn
  needs the position-independent `qemu-i386` from the `qemu-user` package,
  and `vm.mmap_min_addr=0`. None of this matters for the BMC procedure: on
  a 32-bit host, the pointers line up anyway.
- **What works: NVIDIA's tools on the BMC.** NVIDIA's offline massflash
  blob, with its scripts run on the BMC under a 32-bit ARM `qemu-i386`,
  flashed first time, in about a minute.

## A first flash without NVIDIA's tools (2026-11-08, beans `gosd-32dw` and `gosd-le3i`)

`jetson-flash` talks to the same CBoot flashing server NVIDIA's `tegradevflash`
does. A first flash of a module NVIDIA's layout isn't on needs nothing more
than a reflash does: just more partitions. What the bench established (node
3; CBoot's console was read, NVIDIA's tools were not traced):

- **Writing the `GPT` partition is the setup step.** No partition table is
  uploaded separately. When CBoot's server receives a write named `GPT`, it
  erases the boot device, sanitizes the user area, writes the protective MBR
  and both GPTs from the data (the primary GPT from LBA 1, user area only),
  and re-reads them. Every other partition, `PT` included, is then an
  ordinary named write, in NVIDIA's order. The BCT goes last.
- **The eMMC's size comes from OEM command `0x05`.** The GPT needs it. The
  arguments are type 1 (platform details), then sub-command 1 (storage), in
  the 48-byte argument buffer every command shares. The 32-byte reply
  describes the boot device (16384 sectors here: boot0 plus boot1) and then
  the user area (30777344 sectors on this module). Each is
  `{u64 0; u32 sectors; u32 sector size}`.
- **Never send OEM type 9: it provisions the eMMC's RPMB key, once and for
  good.** A probe with zeroed arguments logged `Loading rollback key` and
  `Rollback key provisioned successfully` on this module. An eMMC's RPMB
  authentication key can never be changed after that. Type 15 updates the
  rollback counter, and CBoot skips it on an unfused module. Don't sweep OEM
  types: type 1 is the only one a flash needs.
- **CBoot ignores the partition table's host pointers.** NVIDIA's tools
  build `PT` by dumping raw 32-bit x86 structs, heap pointers included. A
  table carrying string offsets in those words passed
  `PT crc32 and magic check passed`, and the module booted BFS0.
- **CBoot's flashing server finds the boot device's partitions through the
  BCT downloaded in recovery mode.** NVIDIA's flash adds the bootloader
  entries and boot-file info to the BCT (`tegrabct --updatebfsinfo`), then
  downloads that same file in recovery mode and writes it as `BCT` at the
  end. Booted with a BCT that lacks them, the server logs `Tboot Info
  missing from bct`, still takes `GPT`, then refuses `PT` with STATUS 4,
  `Partition PT not found` (bench, 2026-11-09). `jetson-flash` builds the
  final BCT before it can query the eMMC, for this module's 16384-sector
  boot device. The BCT doesn't depend on the user area's size, so only a
  boot device of another size is refused. A module whose user area isn't
  30777344 sectors hasn't been flashed yet.
- **`GPT`'s erase and sanitize took about 0.6 s** on this module, well
  inside the 30 s `jetson-flash` allows each write.
- **Normal warnings after a first flash.** `PT: Partition LNX NOT found`,
  `PT: Partition NCT NOT found` and `Error initializing runtime service
  trusty_fast` appear on every boot, including those after NVIDIA's own
  flashes.

### `jetson-flash flash` end to end (2026-11-09, node 3)

With NVIDIA's kit already cached, `jetson-flash flash hello-b.img` took the
module from an erased eMMC to `gosd hello: listening on [::]:80`. CBoot
logged `Booting BFS0`, `PT crc32 and magic check passed` and `Using BFS PT
to query partitions`. Timed from the program's start, with `--trace`:

| Step | Time |
| --- | --- |
| Recovery mode to CBoot's flashing server | 2.9 s |
| `GPT`, from its data to its STATUS, erase and sanitize included | about 1.4 s |
| The boot device's partitions | about 14 s |
| `APP` (256 MiB), from the write command to its STATUS | 32 s (about 8 MB/s) |
| Each flush, to its STATUS | 0.27 s |
| `BCT`, after the first flush | 0.9 s |
| Total, to the cold boot | 63 s |

The first run also downloads and unpacks NVIDIA's kit, which took under 6
minutes on the BMC, in 17 MB of RSS.

- **CBoot's STATUS messages carry stray bytes** after `GPT`, `APP`, `BCT`
  and the flushes (`"\x96"`, `":\xbd…"`), apparently an uninitialised
  buffer. The code is 0, and only the code means anything.
- **With no BCT, the boot ROM falls back to USB recovery by itself.** A
  flash that stopped just after the `GPT` write left the boot device
  erased, the user area sanitized and fresh GPTs, with nothing else
  written. A plain power-on, with the strap released
  (`tpi usb device -n 3 --bmc`), printed nothing on the console and brought
  up `0955:7f21` on the BMC's USB. As a control, the same power-on after a
  complete flash booted the app, with no recovery device. So a flash
  interrupted after the `GPT` erase can be re-run without the strap. This
  is observed, not guaranteed. A power-on part-way through the boot
  device's writes, an erase cut short and a torn BCT haven't been seen, and
  the strap remains their way back. It also differs from the module straight
  from the distributor above, which showed no recovery device; why isn't
  known.

## Load-address budget: the initramfs is the tight one

The eMMC module runs the same `p3450-0000` U-Boot build as the dev kit, and
it loads at fixed addresses (`tegra210-common.h` lines 53-60). Tegra210
doesn't use the `calculated_vars` mechanism the later SoCs do.

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
`gosd build` refuses a jetson-nano image whose initramfs exceeds this
ceiling, at build time, rather than leaving the failure to the device.

Measured initramfs sizes:

- `examples/hello` built for this board: 10.4 MiB.
- `sattrack` and `usbwebsite`, measured on qemu-virt: 11.5-11.7 MiB.
- Any `--ingress cloudflared` build: about 27 MiB, which `gosd build`
  refuses.

The `u-boot-preboot.scr` hook in `p3450-0000.h` can't lift the limit. It
looks like it could, but `CONFIG_USE_PREBOOT` is off in
`p3450-0000_defconfig`, so `main_loop` never runs the hook.

## What NVIDIA's U-Boot does to our DTB

Before booting, `ft_system_setup` (`arch/arm/mach-tegra/dt-setup.c`)
edits the DTB we supply:

- **GPU:** it enables the GM20B node.
- **Copied nodes:** it copies these nodes from CBoot's DTB:
  - `/memory@80000000`, which gives the real RAM size (useful).
  - `/external-memory-controller@7001b000`, though it skips this one with a
    benign "phandle mismatch" (seen on the bench).
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

The U-Boot facts here come from NVIDIA's L4T U-Boot source as imported by
OE4T,
[branch `l4t/l4t-r32.6.1-v2020.04`](https://github.com/OE4T/u-boot-tegra/tree/46e4604c78d3804ccd4cf9624460a86ea5318a61).
The bench's `printenv` agreed with them for R32.7.6.

## Follow-ups

- **Header I2C and SPI.** On a Turing Pi 2, which header pins reach a
  node's I2C and SPI buses is the carrier's business. Either way, the I2C
  buses (`i2c@7000c000`, `i2c@7000c400`) and SPI buses (`spi@7000d400`,
  `spi@7000d600`) are disabled in `tegra210.dtsi`, and the p3450 DTS
  doesn't enable them. They need the same kind of kernel-build DTS patch as
  the eMMC. The drivers are already built: I2C_TEGRA is required for the
  PMIC anyway, and SPI_TEGRA114 comes from defconfig.
- **Lifting the 14 MiB initramfs ceiling** (see the budget section above).
- **USB gadget.** It's claimed from the DTS and the fleet's config, but
  hasn't been verified on hardware. Given what `ums` showed above, a
  Turing Pi 2 may not deliver it.
