# Pi CM4 kernel

A trimmed, module-free **arm64** kernel for the Raspberry Pi Compute Module 4
(Lite, no-wireless variant), built from
[raspberrypi/linux](https://github.com/raspberrypi/linux) at the same pinned
commit as `build/boards/pi-zero-2w`, `build/boards/pi-zero-w`, and
`build/boards/pi-3b` — the Pi fleet pin, never a single-board bump.

## What's here

- `manifest.json` — pinned third-party blobs: GPU boot firmware
  (bootcode.bin/start.elf/fixup.dat, same raspberrypi/firmware tag as every
  other Pi board). No `wifiFirmware` group: this module has no WiFi/BT
  hardware at all — the first Pi board GoSD ships with none.
- `kernel.fragment` — the config fragment applied on top of
  `bcm2711_defconfig` via `scripts/kconfig/merge_config.sh`. Its header
  documents every deliberate difference from pi-3b's fragment: native GENET
  Ethernet instead of a USB-hosted chip, the BCM2711 iProc SDHCI storage
  controller instead of BCM2835's sdhost, a compiled-in (but not yet
  claimed-supported) USB gadget stack, and no WiFi/BT block. Embedded (via
  `kernelfragment.go`) into `internal/kernelspec.KernelSpec`.
- `kernel.config` — the full `.config` a real build produces, committed for
  reference and cross-build diffing (bean gosd-u5yz). Generated, never
  hand-edited; regenerate via `gosd build-kernel --board pi-cm4 -o out/` and
  copy `out/kernel.config` here.

The kernel and device tree blob are built by
`gosd build-kernel --board pi-cm4`, which drives a
`docker.io/library/debian:bookworm` container using `aarch64-linux-gnu-gcc`
from `internal/kernelspec`'s declarative spec — no board-specific shell
script.

## Building locally

Requires only Docker (`gosd build-kernel` drives it, not the host toolchain):

```sh
go run ./cmd/gosd build-kernel --board pi-cm4 -o out/
```

Outputs land in `out/`:

- `kernel8.img` — the arm64 kernel `Image`, named as the Pi boot firmware
  expects for a 64-bit board (`arm_64bit=1` in `config.txt`)
- `bcm2711-rpi-cm4.dtb` — the device tree blob for this board (the official
  CM4 IO Board's DTS — see the board notes below on carrier fidelity)
- `kernel.config` — the `.config` this run actually used
- `source.json` — upstream repo/commit and config path, for GPL provenance

Build time is roughly 20-60 minutes depending on host CPU; `gosd
build-kernel` content-addresses local builds and skips the container
entirely on a cache hit (see `internal/kernelbuild`).

## Base defconfig: `bcm2711_defconfig`

The same single arm64 defconfig pi-zero-2w and pi-3b already build against —
see `build/boards/pi-zero-2w/README.md` for the `bcmrpi3_defconfig` deletion
history. The CM4's SoC, BCM2711, is squarely inside the hardware range this
defconfig targets.

## Board notes: carrier fidelity and open questions

This board profile ships `bcm2711-rpi-cm4.dtb` — upstream's DTS for the
official Raspberry Pi CM4 IO Board, the closest available match for any
third-party CM4 carrier including Turing Pi 2's node slot. Two things are
genuinely uncharacterized until hardware bring-up (bean gosd-5trv) proves
them one way or another:

- **USB gadget mode.** The CM4's dwc2 controller is compiled in (see
  `kernel.fragment`'s header), but whether Turing Pi 2's node-1 carrier
  routes it to an accessible port at all is unknown — deliberately left as
  "?" rather than guessed at (epic gosd-7676).
- **The onboard status LED.** Whether this carrier wires an LED to the SoC
  GPIO the way a full Pi board does is unverified; no DTS patch has been
  written for it (unlike pi-zero-2w/pi-zero-w/pi-3b's LED-retain-state
  patches) because there may be no node to patch.
