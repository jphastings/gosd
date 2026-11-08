# Putting an app onto a Jetson Nano eMMC module

GoSD's `jetson-nano` board is NVIDIA's 16GB eMMC production module
(P3448-0002, NVIDIA part 900-13448-0020-000). It has no memory card slot,
and NVIDIA's whole boot chain lives on the same eMMC as everything else, in
NVIDIA's own partition layout. A module straight from a distributor has
nothing on it at all.

`jetson-flash` puts an app on it over the module's USB recovery port. It
erases the whole eMMC, then writes NVIDIA's boot chain, built on your
machine from NVIDIA's L4T kit, with your app's boot partition as `APP`. No
NVIDIA tool runs. Every flash is the same, whether the module is new or
already runs an app. It never touches the NVMe drive, where `/data` lives.

Don't reach for `tpi flash`. It writes a whole disk image over the eMMC,
which wipes out NVIDIA's boot chain, and it can't speak NVIDIA's flashing
protocol anyway.

## What you'll need

- The module in a Turing Pi 2 node, flashed from the BMC (proven on a v2.4
  board with BMC firmware 2.3.4). Any Linux PC cabled to the module's
  recovery USB port works too, run as root.
- An NVMe drive in that node's M.2 slot, for `/data`. GoSD formats it on
  first boot only if it's blank, and never erases one that holds anything
  else. Flashing never touches it, so `/data` survives every reflash (see
  [where `/data` lives](runtime.md#persistent-storage-data)).
- Your app's `.img`, built with `gosd build --board jetson-nano`.
- On a Turing Pi 2, a microSD card in the BMC's own slot, mounted at
  `/mnt/sdcard`, to hold the image: the BMC's internal storage is a 24 MB
  overlay plus `/tmp` in 58 MB of RAM.

## 1. Copy `jetson-flash` and the image to the BMC

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./cmd/jetson-flash
scp jetson-flash myapp-jetson-nano.img root@turingpi.local:/mnt/sdcard/
```

For a Linux PC, build with its own `GOARCH` (`arm64` or `amd64`).

## 2. Put the node into recovery mode

On the BMC (`tpi` there needs no login). Wait between off and on: a
back-to-back cycle doesn't always reset a Jetson.

```sh
tpi usb flash -n 4 --bmc
tpi power -n 4 off
sleep 10
tpi power -n 4 on
```

## 3. Flash

```sh
/mnt/sdcard/jetson-flash flash /mnt/sdcard/myapp-jetson-nano.img
```

```
jetson-flash builds the module's boot chain from NVIDIA's L4T kit, which it downloads from NVIDIA. NVIDIA's licence applies:
  https://developer.nvidia.com/downloads/embedded/l4t/r32_release_v7.6/t186/tegra_software_license_agreement-tegra-linux.txt
Do you accept it? [y/N] y
== downloading NVIDIA's L4T kit (266 MiB) from https://developer.download.nvidia.com/embedded/L4T/r32_Release_v7.6/T210/Jetson-210_Linux_R32.7.6_aarch64.tbz2
   10% of 266 MiB
   …
== talking to the boot ROM
== downloading the BCT
== downloading ebt and rp1
== booting CBoot's flashing server
== reading the eMMC's size
== writing GPT, which erases the eMMC
== writing PT
   …
== writing APP (256 MiB)
   10% of 256 MiB
   …
== flushing to storage
== writing BCT
== flushing to storage
== restarting the module
done: the module is booting the new image
```

The kit is streamed, never stored: `jetson-flash` keeps the few MB it
needs under `~/.cache/gosd/l4t`, so later flashes neither download it nor
ask again. Each file is checked against its pinned hash every time it's
read from there.

- `--accept-nvidia-license` accepts the licence without asking, for
  scripts. Without a terminal to ask on, `jetson-flash` stops until you
  pass it.
- `--kit Jetson-210_Linux_R32.7.6_aarch64.tbz2` reads a copy you
  downloaded from
  [NVIDIA's L4T R32.7.6 page](https://developer.nvidia.com/embedded/linux-tegra-r3276)
  instead.
- `--trace` logs every USB transfer.

## 4. Release the recovery strap

```sh
tpi usb device -n 4 --bmc
```

The module has already booted the new image; this lets the next power-on
boot it too.

## Watching it boot

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

## If something goes wrong

- **`no Tegra210 in USB recovery mode (0955:7f21) appeared`.** Check
  `tpi usb status`: the node should be the device and the BMC the host.
  Then repeat step 2.
- **A flash stopped part-way.** The eMMC may be partly written, so it may
  not boot. Put the module back into recovery mode (step 2) and flash
  again: every flash starts from scratch.
- **The same step fails on every attempt.** On the module's console, look
  for `Data recieve from host failure with error 0x120000`. Data isn't
  arriving intact over USB, so the problem is on the host side. Use the
  BMC, and never a VM on a Mac: macOS's USB passthrough lost data in every
  configuration we tried.
