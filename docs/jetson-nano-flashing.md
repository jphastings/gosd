# Putting an app, or another OS, onto a Jetson Nano eMMC module

GoSD's `jetson-nano` board is NVIDIA's 16GB eMMC production module
(P3448-0002, NVIDIA part 900-13448-0020-000). It has no memory card slot,
and NVIDIA's whole boot chain lives on the same eMMC as everything else, in
NVIDIA's own partition layout. A module straight from a distributor has
nothing on it at all.

`jetson-flash` puts an app on it over the module's USB recovery port. It
erases the whole eMMC, then writes NVIDIA's boot chain, built on your
machine from NVIDIA's L4T kit, with your image's filesystem as `APP`. No
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

`jetson-flash` only runs on Linux, so from a Mac you run it on the BMC. Find
the newest `jetson-flash/v…` release on [gosd's releases
page](https://github.com/jphastings/gosd/releases) and download its prebuilt
binary on the BMC, replacing `X.Y.Z` with that version:

```sh
cd /mnt/sdcard
BASE=https://github.com/jphastings/gosd/releases/download/jetson-flash/vX.Y.Z
curl -fLO $BASE/jetson-flash-linux-arm
curl -fLO $BASE/jetson-flash-SHA256SUMS
grep " jetson-flash-linux-arm$" jetson-flash-SHA256SUMS | sha256sum -c -
mv jetson-flash-linux-arm jetson-flash
chmod +x jetson-flash
```

Then copy the image over from your computer:

```sh
scp myapp-jetson-nano.img root@turingpi.local:/mnt/sdcard/
```

On a Linux PC, download `jetson-flash-linux-arm64` or
`jetson-flash-linux-amd64` the same way. Or build from a checkout, with the
`GOARCH` of the machine that will run it (`arm` with `GOARM=7` for the BMC):

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./cmd/jetson-flash
scp jetson-flash myapp-jetson-nano.img root@turingpi.local:/mnt/sdcard/
```

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

It takes about a minute once the kit is cached. The first run also
downloads and unpacks NVIDIA's 266 MiB kit, which took under 6 minutes on a
Turing Pi 2 BMC.

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
   boot device 16384 sectors, user area 30777344
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

## Flashing another OS

`jetson-flash` doesn't care that the image is GoSD's. NVIDIA's U-Boot
boots whatever `APP` holds, so an image of another OS works if `APP` can
take it. The image is one of:

- a GPT disk image: `APP` is its partition named `APP`, otherwise its
  first partition;
- an MBR disk image: `APP` is its first partition;
- a bare ext4 or FAT filesystem image.

The chosen filesystem must be FAT or ext4, which is all NVIDIA's U-Boot
reads, and it must fit in `APP`, which is 14 GiB. `jetson-flash` checks
both before it touches the module. It writes the filesystem as it is, at
about 10 MB/s, so a smaller image flashes faster: a 7 GiB one takes about
12 minutes. Grow it into the rest of `APP` once it's running, with
`sudo resize2fs /dev/mmcblk0p1`.

The OS has to do the rest. It must carry an `extlinux.conf` for U-Boot to
find (`/boot/extlinux/extlinux.conf`, or `/extlinux/extlinux.conf` if
`APP` is its boot partition), and a kernel that runs on the Nano with the
device tree NVIDIA's boot chain provides, or names its own with an `FDT`
line.

### JetPack

NVIDIA's own Ubuntu boots this way. JetPack 4.6.6 (L4T R32.7.6, the
release `jetson-flash`'s boot chain comes from) flashed as a bare ext4
image comes up to a login prompt. To make that image on a Linux machine,
follow NVIDIA's guide to the kit, but put the sample root filesystem in a
loop-mounted ext4 image instead of the kit's `rootfs` directory, and
don't run `flash.sh`. `apply_binaries.sh` wants `qemu-user-static`
installed, even on an arm64 machine.

```sh
cd Linux_for_Tegra
truncate -s 7G system.img
mkfs.ext4 -L APP system.img
sudo mount -o loop system.img rootfs
sudo tar -xpf Tegra_Linux_Sample-Root-Filesystem_R32.7.6_aarch64.tbz2 -C rootfs
sudo ./apply_binaries.sh
sudo ./tools/l4t_create_default_user.sh -u <user> -p <password> -n <hostname> --accept-license
sudo umount -R rootfs
jetson-flash flash system.img
```

`l4t_create_default_user.sh` skips JetPack's first-boot setup, which
otherwise waits for a keyboard and screen. JetPack never grows its root
filesystem by itself on the eMMC module: it only does that when `APP` is
the disk's last partition, and in NVIDIA's eMMC layout `APP` is the
first.

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
