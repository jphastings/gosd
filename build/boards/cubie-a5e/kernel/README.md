# Radxa Cubie A5E kernel

Trimmed, module-free mainline arm64 kernel for the Radxa Cubie A5E (Allwinner
A527, sun55iw3 die). Produces `Image` and the in-tree device tree blob
`sun55i-a527-cubie-a5e.dtb`.

This board's kernel setup mirrors `build/boards/nanopi-zero2/kernel/` (see
that directory's README for the general shape); this file only calls out
what's specific to the A527 — the first Allwinner member of the fleet.

## Building

```sh
go run ./cmd/gosd build-kernel --board cubie-a5e -o out/
```

Requires only Docker. `gosd build-kernel` (bean gosd-07fl) drives everything
— cross toolchain install, kernel source clone, config merge, and compile —
from `internal/kernelspec`'s declarative spec, inside a
`docker.io/library/debian:bookworm` container using the `aarch64-linux-gnu-`
cross prefix. The board is registered (`RegisterInternal`, bean gosd-o7jv),
so `--board cubie-a5e` resolves; it stays internal-only (reachable only via
an explicit `--board=cubie-a5e`, not the default all-boards build) until its
first artifacts release and activation bean land.

**Not yet built as of bean gosd-axtv's code pass**: this fragment and the
`internal/kernelspec` entry are reviewable now; the real Docker build (and
the generated `kernel.config` this README's "Source and configuration"
section describes) is run separately and pushed to this branch before
review — see the bean for status.

Outputs, once built, land in `out/` (gitignored):

- `out/Image` — the kernel image
- `out/sun55i-a527-cubie-a5e.dtb` — the device tree blob
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`
- `out/source.json` — upstream repo/commit and config path, for GPL
  provenance

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the same fleet tag as every other mainline-fleet board
  (`fleetKernelTag` in `internal/kernelspec/kernelspec.go`) — this board
  joins the existing Rockchip-family fleet tag rather than pinning its own.
- `kernel-fragment.config` — the hand-maintained fragment of required
  options, merged onto `make ARCH=arm64 defconfig` via
  `scripts/kconfig/merge_config.sh`. Built by starting from
  `build/boards/nanopi-zero2/kernel/kernel-fragment.config` (closest fleet
  template — same mainline arm64 defconfig base and trim policy) and
  re-deriving every SoC-specific symbol against the pinned kernel source
  tree directly (Kconfig files, `sun55i-a523.dtsi`,
  `sun55i-a527-cubie-a5e.dts`, and the matching driver
  of_device_id/Kconfig tables) — see bean gosd-jpc8's research findings for
  the verified compatible→driver map and gosd-axtv for the fragment-authoring
  pass itself.
- `kernel.config` — not yet committed (see the "Not yet built" note above).
  Once a real `gosd build-kernel --board cubie-a5e` run succeeds, its
  `out/kernel.config` gets copied here, same as every other board.

`internal/kernelspec.go`'s `RequiredY` list asserts that the bean's required
`CONFIG_*` options are still set after `make olddefconfig` resolves
dependencies, and fails loudly if trimming or a kernel version bump silently
dropped one.

A527/sun55i specifics, all pinned by bean gosd-jpc8's research (see that bean
for source links):

- Pinctrl/GPIO is `CONFIG_PINCTRL_SUN55I_A523` (+ `_R` for the RTC-domain
  bank) — sunxi has no separate GPIO driver; pinctrl-sunxi registers the
  gpiochip itself, unlike the Rockchip boards' `CONFIG_GPIO_ROCKCHIP`.
- SD card is `CONFIG_MMC_SUNXI` — SD-boot only, no eMMC node enabled in the
  board DT at this tag.
- Ethernet is `CONFIG_STMMAC_ETH` + `CONFIG_DWMAC_SUN8I` (the sunxi glue,
  not `CONFIG_DWMAC_ROCKCHIP`) + `CONFIG_REALTEK_PHY` (RTL8211F, same PHY
  driver as every other board in the fleet).
- USB gadget is `CONFIG_USB_MUSB_SUNXI` (+ `CONFIG_USB_MUSB_HDRC`,
  `CONFIG_NOP_USB_XCEIV`, `CONFIG_PHY_SUN4I_USB`) — **not** DWC3: the A527's
  Type-C OTG port is a Mentor MUSB controller, not Synopsys DesignWare USB3.
  The board DT already pins `usb_otg`'s `dr_mode` to `"peripheral"`, so
  unlike rock-4se's dwc3 port this needed no DTS patch to become
  gadget-capable. The gadget/configfs function set (ACM/ECM/RNDIS) mirrors
  radxa-zero-3e's fragment — the baseline set every gadget-capable board in
  this fleet carries.
- USB host is `CONFIG_USB_EHCI_HCD_PLATFORM` + `CONFIG_USB_OHCI_HCD_PLATFORM`
  for the board's two enabled host ports (`ehci0`/`ohci0`, `ehci1`/`ohci1`,
  the latter backing the USB-A port) — the A523 has no xHCI/USB3 host
  controller node at this kernel tag, so no separate USB3 host symbol is
  needed.
- The SD card's power rail is a PMIC regulator, not always-on: `mmc0`'s
  `vmmc-supply` is the AXP717's `cldo3` output. `CONFIG_MFD_AXP20X_I2C` +
  `CONFIG_REGULATOR_AXP20X` are therefore a hard boot dependency — without
  them the SD card has no power and vanishes mid-boot, not merely "loses a
  peripheral". Both the AXP717 (main rails, 0x34) and AXP323 (secondary
  rails, 0x36) PMICs sit on `r_i2c0` (`CONFIG_I2C_MV64XXX`, same driver as
  every other I2C bus on this SoC).
- RTC is `CONFIG_RTC_DRV_SUN6I`.
- Console UART is `CONFIG_SERIAL_8250_DW` (same driver as the Rockchip
  boards) at **115200 baud**, not the Rockchip boards' 1500000 — see
  `internal/boards/cubiea5e`'s `defaultConsoleBaud`.

Every Rockchip-specific option the nanopi-zero2 template fragment carried
(`CONFIG_ARCH_ROCKCHIP`, the `dw_mmc`/`dwcmshc` storage pair,
`CONFIG_GPIO_ROCKCHIP`, `CONFIG_I2C_RK3X`, `CONFIG_SPI_ROCKCHIP`,
`CONFIG_DWMAC_ROCKCHIP`, `CONFIG_PHY_ROCKCHIP_*`) is dropped rather than
carried over — none of it applies to the A527.

## Device-tree patches

None. This board has no `patches/` directory: peripheral enablement follows
the same per-SoC, no-runtime-overlay convention as the Rockchip boards
(pinned U-Boot lacks `OF_LIBFDT_OVERLAY`), but at this kernel tag
`sun55i-a523.dtsi` has **no SPI node at all** — there's nothing for a header
SPI patch to flip to `"okay"`, unlike the Rockchip boards' `spidev`
enablement patches. Header I2C is deferred to a post-bring-up follow-up
(locked decision, bean gosd-axtv), even though the controller nodes exist,
to keep this pass scoped to what gosd-jpc8's research already verified.
Revisit both on a future fleet kernel tag bump.

## Updating the pinned kernel version

See `../../radxa-zero-3e/kernel/README.md`'s "Updating the pinned kernel
version" section — `fleetKernelTag` is shared across every mainline-fleet
board (radxa-zero-3e, nanopi-zero2, rock-4se, qemu-virt, and this board), so
bump them together.

## Known limitations

Not yet build-tested (bean gosd-axtv's code pass lands the spec/fragment
first; the real `gosd build-kernel` run and resulting `kernel.config` +
config audit follow separately — see the bean) and not boot-tested on
hardware (bring-up is a later bean under epic gosd-h1wv). Do not treat this
fragment as proven until a real `gosd build-kernel --board cubie-a5e` run
succeeds, its `kernel.config` is committed, and the config has been audited
for defconfig surprises (the Pi-trap lesson in the repo root `CLAUDE.md`:
`=m`-promoted phantom drivers, gadget-zoo/hwsim contention, silently-assumed
firmware cmdline injection).
