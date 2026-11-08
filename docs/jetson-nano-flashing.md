# Putting an app onto a Jetson Nano eMMC module

GoSD's `jetson-nano` board is NVIDIA's 16GB eMMC production module
(P3448-0002, NVIDIA part 900-13448-0020-000). It has no memory card slot,
and its boot firmware isn't in a separate flash chip either: NVIDIA's whole
boot chain lives on the same eMMC as everything else, in NVIDIA's own
partition layout. A module straight from a distributor has nothing on it at
all.

So putting an app onto it is one flash that writes NVIDIA's boot chain *and*
your app's boot files. This guide does that from a Turing Pi 2's BMC with
NVIDIA's own flashing tools. Once the files are ready, the flash itself
takes about a minute.

> **Status:** GoSD's kernel boots from the eMMC this way, but gosd-init
> can't run apps yet. It needs the eMMC enabled in GoSD's device tree
> (bean `gosd-wxy7`) and `/data` on an NVMe drive (bean `gosd-j18s`). A
> single Go tool that replaces this whole procedure is bean `gosd-32dw`.

Don't reach for `tpi flash`. It writes a whole disk image over the eMMC,
which wipes out NVIDIA's boot chain, and it can't speak NVIDIA's flashing
protocol anyway.

## What you'll need

- The module in a Turing Pi 2 node. This was proven on a v2.4 board, in
  node 4, with BMC firmware 2.3.4.
- **A microSD card in the BMC's own slot**, which the BMC mounts at
  `/mnt/sdcard`. The BMC's internal storage is far too small: a 24 MB
  overlay, plus `/tmp` in 58 MB of RAM. You need room for:
  - your app's boot partition (256 MiB unless it was built with a
    different `--boot-size`);
  - about 100 MB of NVIDIA files;
  - the compressed blob while you unpack it (about 55 MB).

  That came to **358 MB** for a default-sized app.
- **A Linux machine to prepare the files**, either x86_64 or arm64. It
  never touches the Jetson. On a Mac, an arm64 Linux VM is fine, and it
  needs no USB passthrough.
- Your app's `.img`, built with `gosd build --board jetson-nano`.

## 1. Prepare NVIDIA's signed flash blob

NVIDIA's flashing tools can build a "massflash blob" offline: a portable,
signed bundle of everything a flash writes, plus NVIDIA's scripts that
write it. No Jetson needs to be connected. The details are in
`README_Massflash.txt` in NVIDIA's download.

On the Linux machine:

```sh
wget https://developer.download.nvidia.com/embedded/L4T/r32_Release_v7.6/T210/Jetson-210_Linux_R32.7.6_aarch64.tbz2
sudo tar -xpf Jetson-210_Linux_R32.7.6_aarch64.tbz2
cd Linux_for_Tegra

# Your app's FAT boot partition becomes APP, partition 1 of NVIDIA's layout.
# GoSD images start it 16 MiB in; count is its size in MiB (--boot-size).
sudo dd if=/path/to/myapp-jetson-nano.img of=bootloader/system.img bs=1M skip=16 count=256

sudo BOARDID=3448 BOARDSKU=0002 FAB=401 BOARDREV=G.0 FUSELEVEL=fuselevel_production \
  ./nvmassflashgen.sh -r jetson-nano-devkit-emmc mmcblk0p1
```

That leaves `mfi_jetson-nano-devkit-emmc.tbz2` (about 55 MB) in
`Linux_for_Tegra`. A few notes on those commands:

- R32.7.6 is NVIDIA's final release for this module.
- `-r` tells NVIDIA's tooling to use your boot partition as `APP`, instead
  of building NVIDIA's Ubuntu.
- The board values are the module's own. Ours reported
  `pm board id is 3448-0002-401` on its console. NVIDIA's README lists
  `FAB=400`; any value from 300 upwards selects the same device tree.

**On an arm64 Linux host**, NVIDIA's tools still run: every one of them is
a static 32-bit x86 program, so Ubuntu's stock user-mode emulation is
enough. Before the commands above, run:

```sh
sudo apt-get install -y qemu-user-static binfmt-support lbzip2 python3 \
  libxml2-utils lz4 device-tree-compiler uuid-runtime cpio
```

## 2. Copy everything to the BMC

The BMC runs NVIDIA's tools too, under a 32-bit ARM build of the same
emulator. It's a single static file, `usr/bin/qemu-i386-static` inside
Debian's or Ubuntu's `qemu-user-static` package for `armhf`:

```sh
apt-get download qemu-user-static:armhf      # needs: sudo dpkg --add-architecture armhf
dpkg-deb -x qemu-user-static_*_armhf.deb qemu-armhf
```

Then copy the blob, the emulator and
[the BMC flashing script](../scripts/jetson-nano-flash-from-bmc.sh) into
one folder on the BMC's card, and unpack the blob there. Unpacking takes
about a minute:

```sh
ssh root@turingpi.local mkdir -p /mnt/sdcard/gosd-jetson
scp mfi_jetson-nano-devkit-emmc.tbz2 qemu-armhf/usr/bin/qemu-i386-static \
  scripts/jetson-nano-flash-from-bmc.sh root@turingpi.local:/mnt/sdcard/gosd-jetson/
ssh root@turingpi.local 'cd /mnt/sdcard/gosd-jetson && bunzip2 -c mfi_jetson-nano-devkit-emmc.tbz2 | tar -xf -'
```

## 3. Flash

Pass the node number and the unpacked folder:

```sh
ssh root@turingpi.local /mnt/sdcard/gosd-jetson/jetson-nano-flash-from-bmc.sh 4 \
  /mnt/sdcard/gosd-jetson/mfi_jetson-nano-devkit-emmc
```

The script:

1. Wraps NVIDIA's three x86 tools so they run under the emulator.
2. Powers the node off, holds its recovery strap with `tpi usb flash -n 4
   --bmc`, and powers it back on. The module then waits in recovery mode as
   USB device `0955:7f21`, on the BMC's own USB port.
3. Runs NVIDIA's `nvmflash.sh`, which prints every stage as it succeeds:
   1. Boot ROM communication.
   2. Sending the BCTs and bootloader.
   3. Booting recovery.
   4. Flashing every partition, with `APP` among them.
   5. Writing the BCT last. Until it lands, a half-written eMMC never
      boots, so an interrupted flash can simply be run again.
   6. Rebooting.
4. Releases the recovery strap, so the next power-on boots from the eMMC.

`tpi` on the BMC itself needs no login.

## 4. Watch it boot

The module's serial console doesn't reach the BMC's node console, so
`tpi uart` shows nothing. It's only on that node's **"UART2 DEBUG"
header**, on the top of the board near the BMC's buttons, at 3.3 V,
115200 8N1. Wire the header's TX to your adapter's RX, plus GND, and
**leave the adapter's TX disconnected**. With it attached, flashing failed
in recovery mode at the bootloader send until it was removed.

A good boot shows, in order:

1. NVIDIA's boot stages (`[L4T TegraBoot]`, then CBoot).
2. `U-Boot 2020.04-g904285e3a7`.
3. `Found /extlinux/extlinux.conf`.
4. Linux starting with GoSD's command line.

## Reflashing a later image without NVIDIA's tools

Once a module has had NVIDIA's layout from the flash above, a later image
only needs `APP` rewritten. `jetson-flash` does that from the BMC, and
needs no emulator. It writes nothing else, so a module that has never had
NVIDIA's layout is refused with a pointer back to this guide.

1. Keep the unpacked blob from the first flash.
2. Build `jetson-flash` for the BMC and copy it over, with the new `.img`:

   ```sh
   CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./cmd/jetson-flash
   ```

3. Put the node into recovery mode. Wait between off and on: a back-to-back
   cycle doesn't always reset a Jetson.

   ```sh
   tpi usb flash -n 4 --bmc
   tpi power -n 4 off
   sleep 10
   tpi power -n 4 on
   ```

4. Flash:

   ```sh
   ./jetson-flash flash myapp-jetson-nano.img /mnt/sdcard/gosd-jetson/mfi_jetson-nano-devkit-emmc
   ```

   It prints a line per step, and ends with `done: the module is booting
   the new image`. Add `--trace` to log every USB transfer.
5. Release the recovery strap:

   ```sh
   tpi usb device -n 4 --bmc
   ```

## If something goes wrong

- **"the Nano never appeared in recovery mode".** Check `tpi usb status`.
  The node should be the device and the BMC the host.
- **The same stage fails on every attempt.** On the module's console, look
  for `Data recieve from host failure with error 0x120000`. That means data
  didn't arrive intact over USB, so the problem is on the host side. Use
  the BMC, not another computer.
- **Don't flash from a VM on a Mac.** We tried hard. Across five
  configurations, about 30 attempts produced one complete flash:
  - an x86 VM, on emulated USB 3 and USB 2, with and without pipelining;
  - UTM's own USB sharing, with x86 and with arm64 guests;
  - an arm64 VM at native speed.

  The module kept reporting that data didn't arrive. The common factor is
  macOS's USB passthrough into the VM.
- **A failed flash leaves the eMMC partly erased.** That's harmless: the
  module still enters recovery mode on request, so just run the flash
  again.
