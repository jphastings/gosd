# Pi 3B kernel

A trimmed, module-free **arm64** kernel for the Raspberry Pi 3B (BCM2837),
built from [raspberrypi/linux](https://github.com/raspberrypi/linux) at the
same pinned commit as `build/boards/pi-zero-2w` and `build/boards/pi-zero-w` —
the Pi fleet pin, never a single-board bump (bean `gosd-ypg1`, epic
`gosd-xhc3`).

## What's here

- `manifest.json` — pinned third-party blobs: GPU boot firmware
  (bootcode.bin/start.elf/fixup.dat, same raspberrypi/firmware tag as the
  other Pi boards) and the BCM43438 WiFi firmware (the Cypress 43430 blob set,
  the same underlying bytes as pi-zero-w, under
  `raspberrypi,3-model-b` alias names — see the manifest's `notes` field).
- `kernel.fragment` — the config fragment applied on top of
  `bcm2711_defconfig` via `scripts/kconfig/merge_config.sh`. Based on
  `pi-zero-2w`'s trim with two deliberate differences (documented in the
  fragment header): the USB gadget block is cut outright (the SoC's USB port
  is hard-wired through the onboard LAN9514 hub, so no UDC can ever exist),
  and a USB HOST + wired Ethernet block is asserted instead
  (`CONFIG_USB_DWCOTG`, `CONFIG_USB_NET_SMSC95XX` and friends — the LAN9514
  is this board's headline feature). It also carries
  `CONFIG_SERIAL_8250_RUNTIME_UARTS=1`, the gosd-md4w serial-console fix,
  from day one. Embedded (via `kernelfragment.go`) into
  `internal/kernelspec.KernelSpec`, the Go-native source of truth for how
  this board's kernel is built.
- `kernel.config` — the full `.config` a real build produces, committed for
  reference and diffing. **Absent until bean gosd-0nl7's first real build
  lands it** (with provenance), matching how the other Pi boards' configs
  were only committed after a green local Docker build.

The kernel and device tree blob are built by
`gosd build-kernel --board pi-3b`, which drives a
`docker.io/library/debian:bookworm` container using `aarch64-linux-gnu-gcc`
from `internal/kernelspec`'s declarative spec — no board-specific shell
script. CI's `pi-3b-kernel` job (added by bean gosd-0nl7) runs the same
command.

## Building locally

Requires only Docker (`gosd build-kernel` drives it, not the host toolchain):

```sh
go run ./cmd/gosd build-kernel --board pi-3b -o out/
```

Outputs land in `out/`:

- `kernel8.img` — the arm64 kernel `Image`, named as the Pi boot firmware
  expects for a 64-bit board (`arm_64bit=1` in `config.txt`)
- `bcm2710-rpi-3-b.dtb` — the device tree blob for this board (the rpi-tree
  bcm2710-\* naming the firmware loads by board match; the tree also builds a
  mainline-style `bcm2837-rpi-3-b.dtb`, which the firmware does not use)
- `kernel.config` — the `.config` this run actually used
- `source.json` — upstream repo/commit and config path, for GPL provenance

Build time is roughly 20-60 minutes depending on host CPU; `gosd
build-kernel` content-addresses local builds and skips the container
entirely on a cache hit (see `internal/kernelbuild`).

## Base defconfig: `bcm2711_defconfig`

The single arm64 defconfig raspberrypi/linux ships for the BCM2710/2711/2712
families (Zero 2 W, 3, 4, 5, CM variants) — see
`build/boards/pi-zero-2w/README.md` for the `bcmrpi3_defconfig` deletion
history. Verified present at the pinned commit, alongside
`arch/arm64/boot/dts/broadcom/bcm2710-rpi-3-b.dts`.

## Driver notes: Ethernet and serial console

- **Ethernet (LAN9514)**: the hub+ethernet chip sits on the SoC's USB bus
  (`bcm283x-rpi-smsc9514.dtsi`). The stock DTB's usb controller node is
  `compatible = "brcm,bcm2708-usb"`, which binds the rpi tree's downstream
  `dwc_otg` driver (`CONFIG_USB_DWCOTG`) — the mainline `dwc2` driver's
  of_match table has no `bcm2708-usb` entry, so `dwc2` would leave this
  board with no USB (and therefore no Ethernet) at all. Both facts verified
  directly against the pinned source (bean gosd-ypg1).
- **Serial console**: BT holds the PL011, so `serial0` is the mini-UART
  (ttyS0, `CONFIG_SERIAL_8250_BCM2835AUX`). `bcm2711_defconfig` ships
  `CONFIG_SERIAL_8250_RUNTIME_UARTS=0`, relying on firmware cmdline
  injection; the fragment bakes `=1` in instead (bean gosd-md4w's pi-zero-w
  lesson, applied here preemptively).
