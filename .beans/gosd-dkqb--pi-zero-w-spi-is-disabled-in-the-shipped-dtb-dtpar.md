---
# gosd-dkqb
title: 'pi-zero-w SPI is disabled in the shipped DTB: dtparam=spi=on is a silent no-op'
status: todo
type: bug
priority: high
created_at: 2026-07-29T22:02:53Z
updated_at: 2026-07-29T22:02:53Z
---

Found while researching audio (epic gosd-qkbl), unrelated to it.
COMPATIBILITY.md claims SPI works on every board, and
`internal/boards/pizerow/templates/config.txt.tmpl` carries
`dtparam=spi=on` to make it so. On pi-zero-w that line does nothing.

## Evidence

`dtc -I dtb -O dts` on the `bcm2835-rpi-zero-w.dtb` from a real
`gosd build-kernel` run (pinned raspberrypi/linux commit
`63598c83153e19b1f99067ab6df7409de2c111f8`):

- `spi: spi@7e204000 { compatible = "brcm,bcm2835-spi"; ... status = "disabled"; }`
- the DTB contains **zero** `__overrides__` nodes.

`dtparam=<x>=on` is implemented by the Pi firmware patching the DTB's
`__overrides__` block at boot. No `__overrides__`, nothing to patch — the
parameter is accepted and discarded. pi-zero-w is the one board GoSD builds
from the **mainline-style** DTS chain (`bcm2835-rpi-zero-w.dts` ->
`bcm2835.dtsi`/`bcm283x.dtsi`), and that chain has no `__overrides__` node
anywhere in it, unlike the downstream-style `bcm2710-*.dts` files
pi-zero-2w and pi-3b build (which do, including an `audio =` entry that
rewrites `chosen`'s `bootargs`). This is another instance of CLAUDE.md's
"know a Pi DTB's lineage" rule, which already records DMA and USB-gadget
versions of the same trap.

So on pi-zero-w today: `/dev/spidev0.*` never appears and
`examples/spiloopback` cannot work.

I2C is fine, but for an unrelated reason and not because of the dtparam:
`bcm2835-rpi.dtsi` sets `&i2c0`/`&i2c1` to `status = "okay"` outright, and
the built DTB confirms `i2c@7e804000 { status = "okay"; }`. So
`dtparam=i2c_arm=on` in the same template is also a no-op on this board —
it just happens not to matter.

## Fix shape

A DTS patch under `build/boards/pi-zero-w/kernel/patches/` setting
`&spi { status = "okay"; ... }` with the `spi0_gpio7` pinctrl and the two
`spidev` child nodes, in the same shape as the Rockchip boards' SPI patches
(`build/boards/rock-4se/kernel/patches/0002-enable-header-spi.patch`) — the
mainline-style DTB needs the same treatment as a no-overlay Rockchip board.
That makes it a kernel-artifact change, so it takes the tag-first,
bump-second artifacts dance in `docs/artifacts.md`.

Worth auditing the pi-zero-w `config.txt` template at the same time: any
other `dtparam=` line in it is also doing nothing, and should either become
a DTS patch or come out with a comment saying why it can't work here.

## Todo

- [ ] Confirm the same conclusion for any other `dtparam=`/`dtoverlay=` line the pi-zero-w template carries (`--usb-gadget` ships `dtoverlay=dwc2` — does the firmware apply a `.dtbo` to an `__overrides__`-less DTB? Overlays and dtparams take different firmware paths, so verify rather than assume)
- [ ] DTS patch enabling `&spi` + `spidev` children against the pinned commit
- [ ] Verify with `dtc -I dtb -O dts` that the built DTB carries the enabled node
- [ ] COMPATIBILITY.md: the SPI row/footnote is wrong for pi-zero-w until this lands
