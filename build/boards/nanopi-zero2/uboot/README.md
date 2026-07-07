# NanoPi Zero2: mainline U-Boot build

Builds `idbloader.img` and `u-boot.itb` from mainline U-Boot for the
FriendlyElec NanoPi Zero2, using Docker for a reproducible cross-build.

## ⚠️ This pins a release candidate, not a release

`UBOOT_TAG` in `build.sh` is pinned to **`v2026.07-rc5`**, not a final
release. `configs/nanopi-zero2-rk3528_defconfig` (with `CONFIG_USB_GADGET`
and Rockusb enabled) is new in the v2026.07 cycle and wasn't in any prior
tagged release (bean gosd-vcae's research, 2026-07-06). The bean's original
gate said to wait for the final v2026.07 tag; that gate was amended
2026-07-07 to pin the latest available `-rc` now instead, so this board is
hardware-testable sooner -- rc tags are still fully reproducible (a fixed
git tag), just not the long-term-stable release line the project otherwise
prefers.

**Follow-up required**: re-pin `UBOOT_TAG` to the final `v2026.07` release
once it ships (expected ~August 2026) and rebuild/re-release artifacts. This
is tracked as an open checklist item on bean `gosd-f39b` -- do not consider
that item done just because this pipeline works on the rc.

## Build

```sh
./build.sh
```

Requires Docker and `jq` on the host. Output lands in `out/idbloader.img`
and `out/u-boot.itb` (gitignored -- these are build products, not source).

## Pinned inputs

- **U-Boot**: mainline, tag pinned in `build.sh` (`UBOOT_TAG`) -- currently
  an rc tag, see the note above.
- **Defconfig**: `nanopi-zero2-rk3528_defconfig`, plus `bootdelay0.config`
  (sets `CONFIG_BOOTDELAY=0`) merged on top via `scripts/kconfig/merge_config.sh`.
  Verified present at the pinned tag before this pipeline was written (this
  board has its own dedicated defconfig -- no substitution from another
  board's config was needed).
- **rkbin blobs** (DDR-init TPL + BL31): pinned by rkbin repo commit, blob
  path, and sha256 in `../manifest.json`. Fetched at build time, verified
  against the pinned hash, never re-hosted -- see the repo root `CLAUDE.md`
  blob policy. Uses the same rkbin commit already pinned for the Radxa
  Zero 3E (`ecb4fcbe954edf38b3ae037d5de6d9f5bccf81f4`), which also carries
  the RK3528 blobs this board needs:
  - `bin/rk35/rk3528_ddr_1056MHz_v1.13.bin` (DDR-init TPL)
  - `bin/rk35/rk3528_bl31_v1.21.elf` (BL31 / TF-A EL3 firmware)

  The rkbin license (recorded in the manifest) permits these blobs being
  embedded in the `idbloader.img`/`u-boot.itb` we produce.

## Boot chain (for context; the image writer/board profile own actually placing these)

Same layout as the Radxa Zero 3E -- this board uses the same Rockchip
BootROM → idbloader → u-boot.itb → extlinux chain:

1. `idbloader.img` (DDR-init TPL + SPL) written to the SD card at **LBA 64**.
2. `u-boot.itb` (FIT: U-Boot proper + the `rk3528-nanopi-zero2` DTB + rkbin
   BL31) written at **LBA 16384**.
3. U-Boot's distro-boot/bootstd then finds `extlinux/extlinux.conf` on the
   first FAT partition (partition 1) of the SD card and boots from there.

## Known gaps

- Not yet serial-verified on real hardware (U-Boot banner, extlinux discovery
  on partition 1). That requires the bring-up hardware kit and is tracked
  separately -- see bean `gosd-odp7`.
- Pins an rc, not a final release -- see the warning above. Re-pinning to
  the final `v2026.07` tag is an open item on bean `gosd-f39b`.
- USB (host or gadget) is not usable on this board at the currently pinned
  kernel tag regardless of what U-Boot enables -- see
  `build/boards/nanopi-zero2/kernel/README.md`'s "Known limitations" section.
  This defconfig's `CONFIG_USB_GADGET`/Rockusb support is real and upstream,
  it just has nothing to bind to yet on the Linux side.
- **No FDT overlay support**: `nanopi-zero2-rk3528_defconfig` (checked at the
  pinned `UBOOT_TAG`) does not set `CONFIG_OF_LIBFDT_OVERLAY`, same finding
  as the Radxa Zero 3E's defconfig (see that board's README). extlinux's
  `fdtoverlays` directive isn't available, so the 30-pin FPC I2C enablement
  in `../kernel/patches/` is a kernel-build-time DTS patch rather than a
  `.dtbo` -- see bean `gosd-85pt`.
