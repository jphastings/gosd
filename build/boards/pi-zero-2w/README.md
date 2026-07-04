# Pi Zero 2 W kernel

A trimmed, module-free arm64 kernel for the Raspberry Pi Zero 2 W, built from
[raspberrypi/linux](https://github.com/raspberrypi/linux) at a pinned commit.

## What's here

- `build.sh` — builds the kernel and device tree blob inside a
  `docker.io/library/debian:bookworm` container, using
  `crossbuild-essential-arm64` (`aarch64-linux-gnu-gcc`). This works the same
  whether the host is arm64 (the "cross" toolchain is effectively native) or
  amd64 (a real cross toolchain, e.g. CI runners).
- `kernel.fragment` — the config fragment carrying every option the bean
  requires, applied on top of `bcm2711_defconfig` via
  `scripts/kconfig/merge_config.sh`.
- `kernel.config` — the full `.config` this produces, committed for
  reference and diffing across future rebuilds.
- `out/` — build.sh's output directory (gitignored, never committed).

## Building locally

Requires only Docker:

```sh
./build.sh
```

Outputs land in `out/`:

- `kernel8.img` — the kernel `Image`, renamed as the Pi boot firmware expects
- `bcm2710-rpi-zero-2-w.dtb` — the device tree blob for this board
- `generated-kernel.config` — the `.config` this run actually used (should
  match the committed `kernel.config`; a mismatch means either upstream
  Kconfig has moved or `kernel.fragment` was edited without regenerating
  `kernel.config`)

Build time is roughly 20-60 minutes depending on host CPU; there's no
incremental cache between runs (each run clones a fresh shallow copy of the
pinned commit).

## Deviation from the bean: `bcmrpi3_defconfig` no longer exists

The bean names `bcmrpi3_defconfig` as the starting defconfig. That defconfig
was deleted upstream in
[commit `7713244`](https://github.com/raspberrypi/linux/commit/7713244d3baee3493108fb98edd82f5b2042ce48)
("configs: Delete bcmrpi3_defconfig": *"Acknowledge the fact that
bcmrpi3_defconfig is neither used nor supported by us"*) and has been gone
from `arch/arm64/configs` since `rpi-6.12.y`. It does not exist on the
current `rpi-6.18.y` branch this build pins to.

`bcm2711_defconfig` is its direct successor: it's the defconfig
raspberrypi/linux currently maintains for all Broadcom arm64 boards
(BCM2710/2711/2712 — Zero 2 W, 3, 4, 400, CM4, ...) via
`CONFIG_ARCH_BCM2835=y`, which is what actually gates whether
`bcm2710-rpi-zero-2-w.dtb` gets built (see
`arch/arm64/boot/dts/broadcom/Makefile`). `build.sh` and `kernel.fragment`
use `bcm2711_defconfig` instead; see the header comments in both files.

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

grep '^# CONFIG_SOUND is not set\|^# CONFIG_SND is not set\|^# CONFIG_DRM is not set\|^# CONFIG_FB is not set\|^# CONFIG_BT is not set' .config
grep '^CONFIG_CC_OPTIMIZE_FOR_PERFORMANCE=y' .config
```

## Boot testing

Not done here — there's no hardware access from this environment. Boot
testing over serial, coordinated with the bring-up task, is the bean's
remaining unchecked todo.
