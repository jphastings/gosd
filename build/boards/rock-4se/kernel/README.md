# Radxa ROCK 4SE kernel

Trimmed, module-free mainline arm64 kernel for the Radxa ROCK 4SE (Rockchip
RK3399-T). Produces `Image` and the in-tree device tree blob
`rk3399-rock-4se.dtb`.

This board's kernel setup mirrors `build/boards/radxa-zero-3e/kernel/` (see
that directory's README for the general shape); this file only calls out
what's specific to the RK3399-T.

## Building

```sh
go run ./cmd/gosd build-kernel --board rock-4se -o out/
```

Requires only Docker. `gosd build-kernel` (bean gosd-07fl) drives everything
— cross toolchain install, kernel source clone, config merge, and compile —
from `internal/kernelspec`'s declarative spec, inside a
`docker.io/library/debian:bookworm` container using the
`aarch64-linux-gnu-` cross prefix.

**Not yet buildable as of bean gosd-iosp's scaffolding pass**: this board
isn't registered in `internal/boards` yet (that's bean gosd-0vvh), so
`gosd build-kernel --board rock-4se` will fail with "unknown board" until
that lands. Everything under this directory plus the `internal/kernelspec`
entry is reviewable now; the real Docker build (and the generated
`kernel.config` this README's "Source and configuration" section describes)
comes after.

Outputs, once buildable, land in `out/` (gitignored):

- `out/Image` — the kernel image
- `out/rk3399-rock-4se.dtb` — the device tree blob
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`
- `out/source.json` — upstream repo/commit and config path, for GPL
  provenance

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the same fleet tag as every other Rockchip-family board
  (`fleetKernelTag` in `internal/kernelspec/kernelspec.go`).
- `kernel-fragment.config` — the hand-maintained fragment of required
  options, merged onto `make ARCH=arm64 defconfig` via
  `scripts/kconfig/merge_config.sh`. Symbol names were verified directly
  against the pinned kernel source tree's Kconfig files (not assumed from
  documentation) — see bean gosd-iosp for the specific source URLs checked.
- `kernel.config` — not yet committed (bean gosd-iosp is scaffolding-only;
  see the "Not yet buildable" note above). Once a real
  `gosd build-kernel --board rock-4se` run succeeds, its `out/kernel.config`
  gets copied here, same as every other board.

RK3399-T specifics, all pinned by bean gosd-je2r's research (see that bean
for source links and confidence levels):

- SD controller is `dw_mshc` family (`CONFIG_MMC_DW` + `CONFIG_MMC_DW_ROCKCHIP`)
  — the same driver as radxa-zero-3e, despite the different SoC family. RK3399's
  eMMC controller is a *different* IP (Arasan SDHCI) and is out of scope
  (SD-boot only).
- USB PHYs are `CONFIG_PHY_ROCKCHIP_INNO_USB2` (u2phy0/1, same as
  radxa-zero-3e) **plus `CONFIG_PHY_ROCKCHIP_TYPEC`** (tcphy0/1, RK3399's
  USB3 Type-C PHY) — **not** `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY`, which is
  RK3566-only and would be silently wrong here.
- GbE PHY is Realtek RTL8211E family (`CONFIG_REALTEK_PHY`), same Kconfig
  symbol as radxa-zero-3e.
- Adds NVMe/PCIe (`CONFIG_PCI`, `CONFIG_PCIE_ROCKCHIP_HOST`,
  `CONFIG_PHY_ROCKCHIP_PCIE`, `CONFIG_BLK_DEV_NVME`) and exFAT
  (`CONFIG_EXFAT_FS` + `CONFIG_NLS_UTF8`) for the betamin example's
  NVMe/exFAT SSD, and USB mass-storage gadget support
  (`CONFIG_USB_CONFIGFS_MASS_STORAGE`) for its USB-exposed storage — none of
  which radxa-zero-3e's fragment needs.

`internal/kernelspec.go`'s `RequiredY` list asserts that the bean's required
`CONFIG_*` options are still set after `make olddefconfig` resolves
dependencies, and fails loudly if trimming or a kernel version bump silently
dropped one.

## Device-tree patches

`patches/` holds GoSD-authored patches applied (via `patch -p1`, in filename
order) to the cloned kernel tree right after checkout, before configuring or
building. Mainline's `rk3399-rock-4se.dts` doesn't enable every peripheral
GoSD wants on by default; rather than fork-and-maintain the whole DTS, each
patch is a small, targeted diff with a comment explaining why it exists. Each
patch below was verified to apply cleanly (`patch -p1 --fuzz=0`) against the
real `rk3399-rock-4se.dts` fetched at the pinned tag — see bean gosd-iosp for
how.

- `0001-enable-header-i2c.patch` — enables `i2c2`, `i2c6`, and `i2c7`
  (`status = "okay"`; each already has its `pinctrl-0` wired at the
  `rk3399-base.dtsi` node level, unlike radxa-zero-3e's `i2c3` which needed
  `pinctrl-0` added too), the three 40-pin header I2C buses bean gosd-je2r
  confirmed disabled and free of any on-board consumer (physical pins 3/5 =
  i2c7, 27/28 = i2c2, 29/31 = i2c6). A PN532 NFC reader hangs off header I2C
  in the betamin example.
- `0002-enable-header-spi.patch` — enables `spi1` (`status = "okay"`, same
  "pinctrl already wired at the base node" shape as the I2C patch above;
  physical pins 19/21/23/24 = MOSI/MISO/SCLK/CS0) plus a `spidev@0` child
  node with the `rohm,dh2228fv` placeholder compatible spidev itself
  documents (same convention as radxa-zero-3e's `0002-enable-header-spi3.patch`).
- `0003-usb-dwc3-peripheral.patch` — `rk3399-rock-pi-4.dtsi` (included by
  this board's `.dts`) forces both `usbdrd_dwc3_0` and `usbdrd_dwc3_1` to
  `dr_mode = "host"`, so out of the box neither USB port is gadget-capable.
  This patch flips `usbdrd_dwc3_0` to `dr_mode = "peripheral"` so the
  betamin example's USB mass-storage gadget has a port to bind to.
  **`usbdrd_dwc3_0` is a best guess, not a confirmed mapping**: bean
  gosd-je2r's research could not determine from DTS text alone which
  controller is wired to the board's physical hardware host/device switch
  (the shared dtsi treats both dwc3 controllers symmetrically). Confirm at
  bring-up (`dmesg`/`lsusb` while toggling the physical switch, or a
  schematic check) and swap to `usbdrd_dwc3_1` if this guess is wrong.

This *is* a kernel-pipeline change: a real rebuild via `gosd build-kernel`
(Docker) regenerates `rk3399-rock-4se.dtb` with these peripherals enabled.
GoSD's committed artifact releases only ship what's actually compiled (see
the repo root `CLAUDE.md`'s no-rehosting policy), so **this board's DTB
artifact needs a new artifacts release** (`artifacts/vX.Y.Z` tag bump)
before real `gosd build` runs (not using `--artifacts-dir`) pick up the
change — same tag-then-bump dance as every other Rockchip board's DTS patch.

## Updating the pinned kernel version

See `../../radxa-zero-3e/kernel/README.md`'s "Updating the pinned kernel
version" section — `fleetKernelTag` is shared across every Rockchip-family
board (radxa-zero-3e, nanopi-zero2, qemu-virt, and this board), so bump them
together, then re-check that every board's `patches/*.patch` still applies
cleanly against the new tag's DTS.

## Known limitations

Not yet build-tested (bean gosd-iosp is a scaffolding-only pass — see the
"Not yet buildable" note above) and not boot-tested on hardware. Do not
treat this fragment/these patches as proven until a real
`gosd build-kernel --board rock-4se` run succeeds and its `kernel.config` is
committed.

Two open questions, both flagged in bean gosd-iosp for bring-up:

- Which physical USB port `usbdrd_dwc3_0` maps to (see
  `0003-usb-dwc3-peripheral.patch` above).
- The 40-pin header pin-number mapping (I2C pins 3/5/27/28/29/31, SPI pins
  19/21/23/24) is sourced from Radxa's own docs/wiki, not an opened
  schematic — worth a quick re-check if a peripheral doesn't enumerate.
