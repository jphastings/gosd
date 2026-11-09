# Artifact releases

Release notes for the board artifacts GoSD builds and publishes: the kernels
and U-Boot builds compiled from the pinned sources in `build/boards/*`, tagged
`artifacts/vX.Y.Z` and attached to a GitHub release.

These are versioned separately from the `gosd` CLI, because a board's kernel or
bootloader can change without the CLI changing and vice versa. A CLI release
pins which artifact release it downloads (`internal/artifacts.Version`), and
that pin is bumped in a follow-up PR after the artifacts release exists — see
the artifacts documentation for the full tag-first, bump-second procedure.

This file is maintained by knope from the change files in `.changeset/`; new
versions are added below this heading.
## 0.10.7 (2026-11-09)

### Features

#### Jetson Nano kernel is now published in artifacts releases

The Jetson Nano's compiled kernel is now attached to artifacts releases,
alongside every other board's. It boots from the module's onboard eMMC,
with NVMe support built in. The board isn't buildable via `gosd build` yet;
that comes in a later step.

## 0.10.6 (2026-11-08)

### Fixes

#### Mainline-fleet kernels move to Linux 6.18.60

The Radxa Zero 3E, ROCK 4SE, Turing RK1, NanoPi Zero2, Cubie A5E and Jetson Nano kernels (and the internal qemu-virt one) are rebuilt from Linux v6.18.60, up from v6.18.37. This is the same 6.18 long-term line, so it carries 23 point releases of upstream stable fixes and no change in supported hardware.

#### The Jetson Nano kernel finds the eMMC module's eMMC

The Jetson Nano's device tree now describes the 16GB eMMC module rather than the SD-card developer kit: the eMMC controller is enabled and always appears as `/dev/mmcblk0`, so gosd-init can mount its boot partition from NVIDIA's `APP` partition. The developer kit's SD slot and M.2 Key E SDIO controllers, which the eMMC module doesn't have, are switched off. The kernel also keeps NVMe support built in, for the drive `/data` will live on.

## 0.10.5 (2026-11-01)

### Features

#### vsock for hosting VMs in every arm64 kernel

Every arm64 board's stock kernel now has AF_VSOCK and vhost-vsock (`/dev/vhost-vsock`), so an app can run QEMU/KVM guests that talk to it over vsock. The Pi 3B, Zero 2 W and CM4 kernels already had it; it's now asserted so a future kernel bump can't drop it. The Pi Zero W is excluded: it has no KVM. The qemu-virt kernel also gets the guest transport.

## 0.10.4 (2026-08-31)

### Features

#### Turing RK1 kernel and U-Boot are now published in artifacts releases

The Turing RK1's compiled kernel and mainline U-Boot (idbloader + FIT with
BL31, rkbin DDR-init blobs) are now attached to artifacts releases,
alongside every other board's. The board itself isn't buildable via `gosd
build` yet — that's a separate, later step — this just gets its compiled
output into a real release for the first time.

## 0.10.3 (2026-08-20)

### Features

#### The status LED's fatal signal now survives kernel shutdown

Every board's status LED device-tree node now carries
`retain-state-shutdown`, so `gpio_led_shutdown()` no longer clears the LED's
level during `device_shutdown()`. Without it, a fatal error's steady on
signal (see [the status LED docs](docs/status-led.md)) went dark the moment
the kernel halted — the same halt that made the signal necessary in the
first place.

The same patch also flips the LED's `default-state` to `"off"` wherever it
wasn't already, so solid-on can only mean a recorded fatal error, never
whatever level the GPIO comes up in before `gosd-init` claims the LED.

One board's ACT LED sits on the Raspberry Pi 3B/3B+'s firmware-owned mailbox
GPIO rather than the SoC's own, so whether its retained level survives past
Linux's own halt depends on the firmware too — bench verification on real
hardware is tracked separately per board.

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

## 0.10.2 (2026-08-17)

### Features

#### The Cubie A5E kernel build now produces a USB-gadget variant device tree

Alongside the board's stock device tree, the build emits one with the
`ehci0`/`ohci0` host controllers disabled, so the USB-C port's phy stays with
the peripheral controller and the board can enumerate as a USB device. The
two are mutually exclusive on this hardware — it has no detection circuitry
to arbitrate between host and peripheral — so the choice is made when an
image is built rather than when it is plugged in.

### Fixes

#### Cubie A5E U-Boot no longer scans USB on every boot

The Radxa Cubie A5E's U-Boot ran an unconditional `usb start` scan before
every boot, purely to detect a USB keyboard that could interrupt autoboot —
on hardware this cost roughly 4.5 seconds of the board's ~9 second U-Boot
phase, scanning four controllers to find nothing. gosd images boot from the
SD card via extlinux and never from USB, so this fragment disables the
preboot scan while leaving USB host, storage and gadget support otherwise
untouched. Measured on hardware: the board's U-Boot phase drops from
9.05s to 4.50s, cutting overall boot time by about a third.

## 0.10.1 (2026-08-16)

### Fixes

#### Cubie A5E images now boot the 1GB RAM variant

The Radxa Cubie A5E's U-Boot now uses DRAM calibration values verified on the
1GB LPDDR4x variant of the board, fixing a U-Boot SPL DRAM-init failure that
previously stopped this variant from booting at all. The 2GB/4GB variants
are not yet hardware-verified and may still have problems; feedback from
anyone running one is welcome (see bean `gosd-84b8`).
