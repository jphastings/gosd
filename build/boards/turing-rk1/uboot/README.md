# Turing RK1: mainline U-Boot build

Builds `idbloader.img` and `u-boot.itb` from mainline U-Boot for the Turing
RK1, using Docker for a reproducible cross-build.

## Build

```sh
./build.sh
```

Requires Docker and `jq` on the host. Output lands in `out/idbloader.img`
and `out/u-boot.itb` (gitignored -- these are build products, not source).

## Pinned inputs

- **U-Boot**: mainline, tag pinned in `build.sh` (`UBOOT_TAG`) -- the same
  v2026.04 tag rock-4se/radxa-zero-3e/cubie-a5e already pin.
  `turing-rk1-rk3588_defconfig` is present and builds cleanly at this tag
  (bean gosd-k4w2's research), so no fleet-wide U-Boot tag bump was needed
  to add this board.
- **Defconfig**: `turing-rk1-rk3588_defconfig`, plus `bootdelay0.config`
  (sets `CONFIG_BOOTDELAY=0` and turns off the `efi_mgr` bootmeth probe)
  merged on top via `scripts/kconfig/merge_config.sh`.
- **rkbin blobs** (DDR-init TPL + BL31): pinned by rkbin repo commit, blob
  path, and sha256 in `../manifest.json`. RK3588 has no open-source DRAM
  init in mainline U-Boot (confirmed against U-Boot's own
  doc/board/rockchip/rockchip.rst), unlike RK3399 (rock-4se) or the A527
  (cubie-a5e) -- this board follows the same rkbin-blob pattern
  radxa-zero-3e and nanopi-zero2 use. Fetched at build time, verified
  against the pinned hash, never re-hosted -- see the repo root `CLAUDE.md`
  blob policy. The rkbin license (recorded in the manifest) permits these
  blobs being embedded in the `idbloader.img`/`u-boot.itb` we produce.

## Boot chain (for context; the image writer owns actually placing these)

1. `idbloader.img` (rkbin DDR-init TPL + SPL) written to eMMC at **LBA 64**.
2. `u-boot.itb` (FIT: U-Boot proper + DTB + rkbin BL31) written at
   **LBA 16384**.
3. U-Boot's distro-boot/bootstd then finds `extlinux/extlinux.conf` on the
   first FAT partition (partition 1) and boots from there.

This is the same two-artifact shape and offsets the rest of the Rockchip
fleet uses. U-Boot's own doc/board/rockchip/rockchip.rst describes RK3588
(and newer Rockchip boards generally) as normally flashed via a single
binman-composed `u-boot-rockchip.bin` at the same `seek=64` offset instead —
confirmed by an actual build that this board's binman config produces all
three files (`idbloader.img` 202752 bytes, `u-boot.itb` 1358336 bytes,
`u-boot-rockchip.bin` 9714176 bytes, the latter just the first two
concatenated with padding). build.sh extracts the two separate pieces,
matching how the image writer already expects two separate raw writes for
every other board in this fleet, rather than adding a third artifact kind
just for this one board.

**No SD path exists for this board at all** — see the parent epic
(`gosd-bntd`) and `docs/turing-rk1-flashing.md`. The RK3588 BootROM doesn't
care what block device these bytes land on, so this doesn't change anything
about the build itself, only how the resulting `.img` reaches the device.

## Known gaps

- Not yet serial-verified on real hardware (U-Boot banner, extlinux
  discovery on partition 1). That happens in the bring-up bean
  (`gosd-hycf`) once the module is on the bench.
- **No FDT overlay support** expected (not yet directly confirmed for this
  defconfig): per-board peripheral toggles would go through a kernel-build-
  time DTS patch rather than a `.dtbo`, per the non-Pi convention -- moot
  for this bring-up since GPIO/peripheral header enablement is out of scope
  (see the epic's locked decisions).
