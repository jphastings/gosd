# Radxa ROCK 4SE: mainline U-Boot build (TF-A from source)

Builds `idbloader.img` and `u-boot.itb` from mainline U-Boot for the Radxa
ROCK 4SE, using Docker for a reproducible cross-build.

## Build

```sh
./build.sh
```

Requires Docker and `jq` on the host. Output lands in `out/idbloader.img`
and `out/u-boot.itb` (gitignored -- these are build products, not source).

## No binary blobs: the RK3399-class template

GoSD's other Rockchip boards (RK3566, RK3528) must fetch two rkbin blobs --
a DDR-init TPL and a BL31 -- because those SoCs have no open-source
equivalents. RK3399 needs neither, so this board's build has **no rkbin
stage at all**:

- **DRAM init**: open-source in U-Boot's own TPL. The defconfig sets
  `CONFIG_TPL=y`, and U-Boot's `doc/board/rockchip/rockchip.rst` rk3399
  recipe (read at the pinned tag) exports no `ROCKCHIP_TPL` -- unlike the
  same doc's rk3308/rk3528/rk3568 recipes, which do.
- **BL31**: compiled from mainline Trusted-Firmware-A inside the same
  Dockerfile (`make PLAT=rk3399 bl31`). TF-A's rk3399 platform additionally
  builds the PMU's Cortex-M0 firmware as a hard dependency of BL31 (it's
  `.incbin`'d into `pmu_fw.S`), which is why the image installs
  `gcc-arm-none-eabi` alongside the aarch64 cross toolchain.

Everything in the output is compiled from pinned sources, recorded in
`../manifest.json`'s `tfa` section (repo, tag, peeled commit, BSD-3-Clause
license note) and `UBOOT_TAG` in `build.sh`. The Dockerfile verifies the
TF-A clone's HEAD against the pinned commit, standing in for the sha256
check the blob-fetching boards do.

Future RK3399-class boards should copy this directory's shape rather than
the radxa-zero-3e/nanopi-zero2 rkbin shape.

## Pinned inputs

- **U-Boot**: mainline, tag pinned in `build.sh` (`UBOOT_TAG`).
- **Defconfig**: `rock-4se-rk3399_defconfig`, plus `bootdelay0.config`
  (sets `CONFIG_BOOTDELAY=0`) merged on top via
  `scripts/kconfig/merge_config.sh`. Single-board defconfig
  (`CONFIG_DEFAULT_DEVICE_TREE="rockchip/rk3399-rock-4se"`, no
  `CONFIG_OF_LIST` runtime model detection -- unlike the shared Radxa Zero
  3E/3W defconfig).
- **TF-A**: repo, tag, and peeled commit pinned in `../manifest.json`.

## Boot chain (for context; the image writer owns actually placing these)

1. `idbloader.img` (open-source TPL + SPL) written to the SD card at **LBA 64**.
2. `u-boot.itb` (FIT: U-Boot proper + DTB + TF-A BL31) written at
   **LBA 16384**.
3. U-Boot's distro-boot/bootstd then finds `extlinux/extlinux.conf` on the
   first FAT partition (partition 1) of the SD card and boots from there.

## Known gaps

- Not yet serial-verified on real hardware (U-Boot banner, extlinux discovery
  on partition 1). That happens in the bring-up bean -- see the parent epic.
- **No FDT overlay support**: `rock-4se-rk3399_defconfig` (checked at the
  pinned `UBOOT_TAG`) does not set `CONFIG_OF_LIBFDT_OVERLAY`, and no merged
  fragment adds it. extlinux.conf's `fdtoverlays` directive isn't available,
  so per-board peripheral toggles go through kernel-build-time DTS patches
  (see `../kernel/patches/`), per the project-wide Rockchip convention.
