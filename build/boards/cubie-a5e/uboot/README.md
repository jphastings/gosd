# Radxa Cubie A5E: mainline U-Boot build (TF-A from source)

Builds `u-boot-sunxi-with-spl.bin` from mainline U-Boot for the Radxa Cubie
A5E (Allwinner A527, sun55iw3 die), using Docker for a reproducible
cross-build.

## Build

```sh
./build.sh
```

Requires Docker and `jq` on the host. Output lands in
`out/u-boot-sunxi-with-spl.bin` (gitignored -- this is a build product, not
source).

Verified locally end-to-end (2026-08): the build produces a
**819137-byte** `u-boot-sunxi-with-spl.bin` -- well under the 16MiB the boot
partition starts at, with the full pre-partition gap minus the 8KiB
BootROM-load offset (`internal/boards/cubiea5e`'s `maxUbootEndBytes` guard) to
spare.

## Boot chain: one file where Rockchip needs two

The sunxi BootROM loads a single file straight from a fixed SD-card byte
offset -- **8KiB (sector 16)** -- and runs it: `u-boot-sunxi-with-spl.bin`, a
FIT image U-Boot's own build packages from SPL, BL31, U-Boot proper, and the
board DTB. There's no separate idbloader/u-boot.itb split like the Rockchip
boards; `internal/boards/cubiea5e` issues one `image.RawWrite` at that offset
instead of two. From there boot proceeds exactly like the Rockchip boards:
U-Boot's distro-boot/bootstd finds `extlinux/extlinux.conf` on the first FAT
partition (partition 1) of the SD card and boots from there.

## No binary blobs, but one fork-pinned source

Like rock-4se, this board's build has **no rkbin-style blob stage**:

- **DRAM init**: open-source in mainline U-Boot's SPL.
  `radxa-cubie-a5e_defconfig` sets `CONFIG_MACH_SUN55I_A523=y`, which selects
  `arch/arm/mach-sunxi/dram_sun55i_a523.c` -- no boot0/blob.
- **BL31**: compiled from source inside the same Dockerfile (`make
  PLAT=sun55i_a523 bl31`).

Unlike rock-4se's TF-A pin, though, the A523's BL31 does **not** come from
upstream Trusted-Firmware-A: mainline TF-A has no `sun55i_a523` platform at
any release (checked as of v2.15.0 and `master` -- bean gosd-jpc8). The
community-standard source is a fork,
[jernejsk/arm-trusted-firmware](https://github.com/jernejsk/arm-trusted-firmware),
branch `a523`. That's someone else's development branch, not a tagged
release, so it can move at any time -- pinning it the way rock-4se pins TF-A
(clone the branch, verify HEAD equals the pinned commit) would break this
build the moment the branch tip moves on. Instead, `../manifest.json`'s
`tfa.commit` is the **authoritative** pin, and the Dockerfile fetches that
commit directly:

```sh
git init && git remote add origin "$TFA_REPO" \
  && git fetch --depth 1 origin "$TFA_COMMIT" \
  && git checkout FETCH_HEAD
```

`tfa.branch` in the manifest is informational only -- it records which
upstream line the pin was taken from, for humans re-pinning later. It is
never used to select what gets built.

**Revisit this pin when mainline TF-A gains a `sun55i_a523` platform.** At
that point this board should move to upstream TF-A at a tagged release, the
same shape as rock-4se's `tfa` section (`tag` + peeled `commit`, clone-and-
verify instead of fetch-by-commit).

Everything in the output is still compiled from pinned sources -- nothing is
fetched as a binary blob, and BSD-3-Clause permits embedding the resulting
`bl31.bin` in the `u-boot-sunxi-with-spl.bin` FIT image we produce and
distribute (see `../manifest.json`'s `license_note`).

## Pinned inputs

- **U-Boot**: mainline, tag pinned in `build.sh` (`UBOOT_TAG`).
- **Defconfig**: `radxa-cubie-a5e_defconfig`, plus `bootdelay0.config` (sets
  `CONFIG_BOOTDELAY=0`), `dram-1gb.config` (overrides four per-chip vendor
  DRAM calibration values, see below) and `skip-usb-scan.config` (silences
  the boot-time USB preboot scan, see below) merged on top via
  `scripts/kconfig/merge_config.sh`. Single-board defconfig
  (`CONFIG_DEFAULT_DEVICE_TREE="allwinner/sun55i-a527-cubie-a5e"`).
- **TF-A**: repo, branch (informational), and commit (authoritative) pinned
  in `../manifest.json`.

## BL31 handoff details

Per U-Boot's sunxi build docs (`doc/board/allwinner/sunxi.rst` at the pinned
tag), the final `make` step passes:

- `BL31=<path>/bl31.bin` -- the **raw binary**, not the `.elf` rock-4se's
  rk3399 build uses. Passing the ELF here silently produces a FIT image with
  no working BL31.
- `SCP=/dev/null` -- the A523 uses no SCP (Cortex-M0 co-processor) firmware,
  unlike some other Allwinner SoCs. Without this, U-Boot's build looks for an
  SCP binary that doesn't exist and prints a warning; pointing it at
  `/dev/null` makes the absence explicit.

No `LD=aarch64-linux-gnu-ld` override: unlike rock-4se's rk3399 TF-A build
(which needs it to avoid a `.pmusram` link-placement bug in TF-A's
clang-linker-driver default since v2.13), the A523 platform's default link
places BL31 correctly with Debian's aarch64-linux-gnu toolchain as-is.

## DRAM calibration: verified on the 1GB variant only

`radxa-cubie-a5e_defconfig` hardcodes one fixed set of
`CONFIG_DRAM_SUNXI_TPR*` values -- per-chip vendor DRAM calibration data --
but the Cubie A5E ships in 1GB/2GB/4GB LPDDR4x variants with different DRAM
chips, and upstream's values only suit whichever reference unit was used to
submit the board. On JP's 1GB board (bean `gosd-6pfn`) upstream's values
fail U-Boot SPL's DRAM init outright (`DRAM test failure at address
0x6fffffc0`; root-caused in bean `gosd-84b8`). `dram-1gb.config` overrides
four of those values with ones read back from a working vendor bootloader
by the Armbian community
([Guation/radxa-cubie-a5e-armbian-build@202f1bf](https://github.com/Guation/radxa-cubie-a5e-armbian-build/commit/202f1bf))
and hardware-verified the same way: rebuilding with only these four values
changed took the board from halting in SPL to a clean `DRAM: 1024 MiB` and a
full boot, first try.

**This is verified on a 1GB board only.** Keeping these values rather than
detuning for hypothetical compatibility is a deliberate decision (bean
`gosd-84b8`, 2026-08-21): they are the only ones anyone has been able to
prove against real silicon. The 2GB/4GB variants ship different DRAM chips
and may still fail SPL DRAM init with these values, which presents as a
brick -- see the board notes in the compatibility matrix, and bean
`gosd-6pfn` if you have one of those variants and want to report back.
Re-deriving values for another variant means reading
`tpr6`/`tpr10`/`tpr11`/`tpr12` back off that
variant's own working vendor bootloader, the same way the Armbian community
did here.

## Boot-time: skipping the unconditional USB preboot scan

On hardware, U-Boot spent ~4.5s of its ~9s boot phase running a `usb start`
scan that finds nothing and is never needed: gosd images boot from mmc via
extlinux and never from USB (bean `gosd-uj4l`). At the pinned `UBOOT_TAG`,
`arch/arm/Kconfig`'s `ARCH_SUNXI` hard-`select`s `CONFIG_CMD_USB`,
`CONFIG_USB_STORAGE` and `CONFIG_USB_KEYBOARD` whenever
`DISTRO_DEFAULTS && USB_HOST` (both true here -- DISTRO_DEFAULTS drives the
mmc/extlinux boot this board depends on, USB_HOST is select'd by the
EHCI/OHCI host-controller drivers) -- a `select` a fragment can't turn off,
and disabling USB_HOST to break the condition would remove real USB host
support along with the scan. The actual trigger is one step further down
the same chain but is an ordinary `default`, not a `select`:
`CONFIG_USB_KEYBOARD` gives `boot/Kconfig`'s `CONFIG_PREBOOT` a default of
`"usb start"`, run unconditionally before the boot-delay countdown so a
keyboard plugged in can interrupt autoboot. `skip-usb-scan.config` overrides
that default to an empty string, which a fragment *can* do -- confirmed by
generating this defconfig's real `.config` at the pinned tag with
`scripts/kconfig/merge_config.sh -m` + `make olddefconfig`: `CONFIG_PREBOOT`
stays empty while `CONFIG_CMD_USB`, `CONFIG_USB_STORAGE`, `CONFIG_USB_HOST`,
`CONFIG_DM_USB` and `CONFIG_USB_GADGET` (this board's MUSB peripheral mode,
used by `--usb-gadget`) are all untouched, and so are `CONFIG_BOOTSTD`,
`CONFIG_DISTRO_DEFAULTS` and `CONFIG_BOOTCOMMAND` (`run distro_bootcmd`) --
the mmc extlinux boot path is unaffected, and USB remains available as a
`distro_bootcmd` fallback target and from the U-Boot command line, just not
auto-run.

**The ~4.5s saving is not yet re-measured.** This fragment was written
without a container runtime or bench access, so it's verified against the
pinned tag's actual Kconfig sources and the real kconfig tooling (see
above), but not through an actual U-Boot rebuild and hardware boot -- see
bean `gosd-uj4l` for the todo to rebuild and re-measure.

## Known gaps

- Serial-verified on real hardware (bean `gosd-6pfn`, 2026-08-16): SPL banner,
  BL31 handoff, U-Boot proper, extlinux discovery on partition 1 and a full
  boot to /app all confirmed on a 1GB board -- with `dram-1gb.config` in
  place, without which SPL never reaches any of it. The 2GB/4GB variants
  remain unverified (see the DRAM calibration section above).
  `skip-usb-scan.config` postdates that session and has not yet been through
  a rebuild + bench boot (see the boot-time section above).
- **No FDT overlay support**: `radxa-cubie-a5e_defconfig` (checked at the
  pinned `UBOOT_TAG`) does not set `CONFIG_OF_LIBFDT_OVERLAY`, and no merged
  fragment adds it. extlinux.conf's `fdtoverlays` directive isn't available,
  so per-board peripheral toggles go through kernel-build-time DTS patches,
  per the project-wide non-Pi convention.
