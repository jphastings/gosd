---
artifacts: minor
---

#### SPI now works on the Raspberry Pi Zero W

The Pi Zero W's SPI0 controller is now enabled directly in its device tree,
giving `/dev/spidev0.0`/`/dev/spidev0.1` at boot the same as every other
board. `config.txt`'s `dtparam=spi=on` has always been a silent no-op on
this board: it's the one GoSD board built from the mainline-style DTS chain
(`bcm2835-rpi-zero-w.dts`), which — unlike the downstream-style DTBs the
other Pi boards use — carries no `__overrides__` node for the Raspberry Pi
firmware's `dtparam` mechanism to patch, so the parameter was accepted and
silently discarded.

`dtparam=i2c_arm=on` in the same file has the identical problem, but it
happens not to matter: I2C is already enabled unconditionally by this
board's own device tree, independent of that line.

Bench verification on real hardware is tracked separately (bean
`gosd-dkqb`).
