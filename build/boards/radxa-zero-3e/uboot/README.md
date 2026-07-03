# Radxa Zero 3E: mainline U-Boot build

Builds `idbloader.img` and `u-boot.itb` from mainline U-Boot for the Radxa
Zero 3E, using Docker for a reproducible cross-build.

## Build

```sh
./build.sh
```

Requires Docker and `jq` on the host. Output lands in `out/idbloader.img`
and `out/u-boot.itb` (gitignored -- these are build products, not source).

## Pinned inputs

- **U-Boot**: mainline, tag pinned in `build.sh` (`UBOOT_TAG`).
- **Defconfig**: `radxa-zero-3-rk3566_defconfig`, plus `bootdelay0.config`
  (sets `CONFIG_BOOTDELAY=0`) merged on top via `scripts/kconfig/merge_config.sh`.
- **rkbin blobs** (DDR-init TPL + BL31): pinned by rkbin repo commit, blob
  path, and sha256 in `../manifest.json`. Fetched at build time, verified
  against the pinned hash, never re-hosted -- see the repo root `CLAUDE.md`
  blob policy. The rkbin license (recorded in the manifest) permits these
  blobs being embedded in the `idbloader.img`/`u-boot.itb` we produce.

## Defconfig coverage finding (3E vs 3W)

The bean asked us to verify that `radxa-zero-3-rk3566_defconfig` actually
covers the 3E, not just the 3W, via runtime device-tree selection. Confirmed
against the pinned U-Boot source:

- The defconfig sets `CONFIG_OF_LIST="rockchip/rk3566-radxa-zero-3w
  rockchip/rk3566-radxa-zero-3e"`, so both DTBs are built into the FIT image
  (`CONFIG_DEFAULT_DEVICE_TREE` / `CONFIG_DEFAULT_FDT_FILE` just pick the 3w
  DTB as the *default*, not the only option).
- `board/radxa/zero3-rk3566/zero3-rk3566.c` reads an ADC channel (SARADC
  channel 1, the board's hardware-ID resistor) and picks between the two
  models by voltage range: 230-270 -> 3w, 400-450 -> 3e.
- `board_fit_config_name_match()` uses that same detection to choose which
  DTB config name to load from the FIT during SPL boot, and
  `rk_board_late_init()` sets the `fdtfile` env var accordingly so the full
  U-Boot (and later extlinux) also point at the right DTB.

So yes: one defconfig, one build, runtime hardware detection -- no separate
3E defconfig or build needed.

## Boot chain (for context; the image writer owns actually placing these)

1. `idbloader.img` (DDR-init TPL + SPL) written to the SD card at **LBA 64**.
2. `u-boot.itb` (FIT: U-Boot proper + both DTBs + rkbin BL31) written at
   **LBA 16384**.
3. U-Boot's distro-boot/bootstd then finds `extlinux/extlinux.conf` on the
   first FAT partition (partition 1) of the SD card and boots from there.

## Known gaps

- Not yet serial-verified on real hardware (U-Boot banner, extlinux discovery
  on partition 1). That requires the bring-up hardware kit and is tracked
  separately -- see the parent bean.
