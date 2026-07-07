# Pi Zero W kernel

A trimmed, module-free **32-bit (armv6)** kernel for the original Raspberry Pi
Zero W, built from [raspberrypi/linux](https://github.com/raspberrypi/linux)
at the same pinned commit as `build/boards/pi-zero-2w` (bean `gosd-s7fk`, epic
`gosd-ajpz`).

## What's here

- `build.sh` — builds the kernel and device tree blob inside a
  `docker.io/library/debian:bookworm` container, using
  `crossbuild-essential-armhf` (`arm-linux-gnueabihf-gcc`). Unlike
  `pi-zero-2w`/`radxa-zero-3e`, this is a genuine cross-compile even on an
  arm64 host: the target is 32-bit armv6, not the host's native
  architecture.
- `kernel.fragment` — the config fragment carrying every option the bean
  requires, applied on top of `bcmrpi_defconfig` via
  `scripts/kconfig/merge_config.sh`. Mirrors `pi-zero-2w`'s fragment
  one-for-one.
- `kernel.config` — the full `.config` this produces, committed for
  reference and diffing across future rebuilds.
- `out/` — build.sh's output directory (gitignored, never committed).

## Building locally

Requires only Docker:

```sh
./build.sh
```

Outputs land in `out/`:

- `kernel.img` — the kernel `zImage`, renamed as the Pi boot firmware expects
  for a 32-bit board (no `arm_64bit=1` in `config.txt`)
- `bcm2835-rpi-zero-w.dtb` — the device tree blob for this board
- `generated-kernel.config` — the `.config` this run actually used (should
  match the committed `kernel.config`; a mismatch means either upstream
  Kconfig has moved or `kernel.fragment` was edited without regenerating
  `kernel.config`)

Build time is roughly 20-60 minutes depending on host CPU; there's no
incremental cache between runs (each run clones a fresh shallow copy of the
pinned commit).

## Toolchain note: `arm-linux-gnueabihf-` targeting armv6

The Zero W's SoC (BCM2835) has a single ARM1176JZF-S core — armv6, with VFPv2
hardware floating point but **not** the Thumb-2/armv7 instruction set. Debian
does not ship an armv6-specific cross toolchain; `crossbuild-essential-armhf`
provides `arm-linux-gnueabihf-gcc`, whose *default* tuning targets armv7.
This is nonetheless the correct, standard toolchain for building upstream and
raspberrypi/linux armv6 kernels: the kernel build sets its own `-march=armv6`/
`-mtune` flags from Kconfig (`CONFIG_ARM_ARCH_6` etc., selected transitively
by `bcmrpi_defconfig`'s `CONFIG_ARCH_BCM2835`/machine choice), which override
the toolchain's default tuning for every object file compiled. The `hf`
(hard-float) ABI matches the ARM1176's VFPv2 unit, so no ABI mismatch results
either. This is the same toolchain raspberrypi/linux's own documented build
instructions use for `bcmrpi_defconfig`.

## Base defconfig: `bcmrpi_defconfig`

`bcmrpi_defconfig` is raspberrypi/linux's armv6 defconfig, covering the
original BCM2835 family (Pi 1, Pi Zero, Pi Zero W) via `CONFIG_ARCH_BCM2835=y`
in `arch/arm/configs`. Verified present at the pinned commit, alongside
`arch/arm/boot/dts/broadcom/bcm2835-rpi-zero-w.dts` (the DTS lives under the
`broadcom/` subdirectory, same reorg that moved the arm64 DTS tree — see
`pi-zero-2w/README.md`'s note on that move).

## Config verification

After `make olddefconfig`, the resulting `.config` must satisfy every
requirement in the bean:

```sh
cd /build/linux   # inside the build container, or against a local checkout
grep '^# CONFIG_MODULES is not set' .config

grep '^CONFIG_DEVTMPFS=y\|^CONFIG_DEVTMPFS_MOUNT=y\|^CONFIG_PROC_FS=y\|^CONFIG_SYSFS=y' .config
grep '^CONFIG_BLK_DEV_INITRD=y\|^CONFIG_RD_ZSTD=y' .config
grep '^# CONFIG_RD_GZIP is not set\|^# CONFIG_RD_BZIP2 is not set\|^# CONFIG_RD_LZMA is not set\|^# CONFIG_RD_XZ is not set\|^# CONFIG_RD_LZO is not set\|^# CONFIG_RD_LZ4 is not set' .config
grep '^CONFIG_VFAT_FS=y\|^CONFIG_NLS_CODEPAGE_437=y\|^CONFIG_NLS_ISO8859_1=y\|^CONFIG_TMPFS=y' .config

grep '^CONFIG_MMC=y\|^CONFIG_MMC_BCM2835=y' .config

grep '^CONFIG_CFG80211=y\|^CONFIG_BRCMFMAC=y\|^CONFIG_RFKILL=y' .config

grep '^CONFIG_INET=y\|^CONFIG_IPV6=y\|^CONFIG_PACKET=y\|^CONFIG_UNIX=y' .config

grep '^CONFIG_USB_DWC2=y\|^CONFIG_USB_DWC2_DUAL_ROLE=y\|^CONFIG_USB_GADGET=y' .config
grep '^CONFIG_USB_CONFIGFS=y\|^CONFIG_USB_CONFIGFS_ACM=y\|^CONFIG_USB_CONFIGFS_ECM=y\|^CONFIG_USB_CONFIGFS_RNDIS=y\|^CONFIG_USB_LIBCOMPOSITE=y' .config

grep '^CONFIG_GPIO_CDEV=y\|^CONFIG_GPIO_CDEV_V1=y\|^CONFIG_I2C_BCM2835=y\|^CONFIG_SPI_BCM2835=y' .config
grep '^CONFIG_SERIAL_8250=y\|^CONFIG_SERIAL_8250_BCM2835AUX=y\|^CONFIG_SERIAL_AMBA_PL011=y\|^CONFIG_SERIAL_AMBA_PL011_CONSOLE=y' .config

grep '^# CONFIG_SOUND is not set\|^# CONFIG_DRM is not set\|^# CONFIG_FB is not set\|^# CONFIG_BT is not set' .config
grep '^CONFIG_CC_OPTIMIZE_FOR_PERFORMANCE=y' .config
```

Every option above survived `make olddefconfig` unchanged. Two symbols from
the reference (`pi-zero-2w`) verification list are absent from this kernel
version's Kconfig entirely (not merely unset), so they're dropped from the
list above rather than asserted:

- `CONFIG_SND` — `CONFIG_SOUND` being off removes the whole sound subsystem
  from the Kconfig tree, so `CONFIG_SND` itself isn't emitted as an
  `is not set` line (same behavior on `pi-zero-2w`'s committed config).
- `CONFIG_EXT3_FS` — this kernel version no longer has a distinct ext3
  driver; ext3 mounting is handled by the ext4 driver (`CONFIG_EXT4_FS`,
  which is off here), so there's no separate symbol to assert either way
  (also true of `pi-zero-2w`'s config).

One casualty carries over from `pi-zero-2w` as well: `CONFIG_DEBUG_KERNEL=y`
survives `olddefconfig` despite `kernel.fragment` asking for it off —
something else selected in this kernel version pulls it back in. This is not
a new regression introduced by the 32-bit build; `pi-zero-2w/kernel.config`
shows the identical survival.

## Boot testing

Not done here — there's no hardware access from this environment. Boot
testing over serial, coordinated with the bring-up task, is the bean's
remaining unchecked todo.
