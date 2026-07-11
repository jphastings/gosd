# NanoPi Zero2 kernel

Trimmed, module-free mainline arm64 kernel for the FriendlyElec NanoPi Zero2
(Rockchip RK3528A). Produces `Image` and the in-tree device tree blob
`rk3528-nanopi-zero2.dtb`.

## Building

```sh
go run ./cmd/gosd build-kernel --board nanopi-zero2 -o out/
```

Requires only Docker. `gosd build-kernel` (bean gosd-07fl) drives everything
— cross toolchain install, kernel source clone, config merge, and compile —
from `internal/kernelspec`'s declarative spec, inside a
`docker.io/library/debian:bookworm` container using the
`aarch64-linux-gnu-` cross prefix, so it produces identical output on an
arm64 host (e.g. Apple Silicon, native container) or an amd64 CI runner
(true cross-compilation). No local kernel source checkout or toolchain
install is needed on the host; there is no board-specific shell script
anymore — `.github/workflows/build-artifacts.yml`'s `nanopi-zero2-kernel`
job runs the exact same command CI-side.

Outputs land in `out/` (gitignored):

- `out/Image` — the kernel image
- `out/rk3528-nanopi-zero2.dtb` — the device tree blob
- `out/kernel.config` — the full `.config` actually used for that build, for
  comparison against the committed `kernel.config`
- `out/source.json` — upstream repo/commit and config path, for GPL
  provenance

## Source and configuration

- Kernel source: mainline stable (`git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git`),
  pinned to the same tag as every other board's kernel pipeline (see
  `fleetKernelTag` in `internal/kernelspec.go`) — one shared kernel version
  across the fleet.
- `kernel-fragment.config` — the hand-maintained fragment of required options
  (SoC, storage, Ethernet, peripherals, and cuts), merged onto
  `make ARCH=arm64 defconfig` via `scripts/kconfig/merge_config.sh`.
- `kernel.config` — the full generated `.config` from the last known-good
  build (header records the source tag/repo/generation method). This is what
  ships in release manifests for GPL compliance; it is not itself fed back
  into the build — `gosd build-kernel` always regenerates from `defconfig` +
  the fragment so the build stays reproducible from source.

`internal/kernelspec.go`'s `RequiredY` list asserts that the bean's required
`CONFIG_*` options are still set after `make olddefconfig` resolves
dependencies, and fails loudly if trimming or a kernel version bump silently
dropped one (formerly `docker-build.sh`'s job, before bean gosd-07fl retired
that script).

## Device-tree patches

`patches/` holds GoSD-authored patches applied (via `patch -p1`) to the
cloned kernel tree right after checkout, before configuring or building —
same mechanism as the Radxa Zero 3E's (see that board's README).

- `0001-enable-header-i2c5.patch` — enables `i2c5` (`status = "okay"`,
  `pinctrl-0 = <&i2c5m0_xfer>`), which mainline leaves disabled with no
  default pinctrl. This is the bus wired to the 30-pin FPC connector's pins
  12/13 (GPIO1_B2/B3), confirmed against FriendlyElec's own schematic
  (`NanoPi_Zero2_2407_SCH.pdf`'s GPIO table) — **not** `i2c1`, which this
  board already enables for its onboard RTC (`hym8563`) but which the
  schematic shows is not routed to the FPC connector at all. The schematic
  also notes the FPC's I2C5 pins need an **external 2.2kΩ pull-up**: unlike
  `i2c1`'s RTC bus, there are no onboard pull-ups on this one. Unlike the
  Radxa Zero 3E, `rk3528.dtsi` doesn't pre-alias any `i2cN` node at the SoC
  level (this board's own `aliases` block has to name `i2c1` explicitly
  already, for the same reason), so the patch also adds an `i2c5 = &i2c5;`
  alias — without it, `/dev/i2c-5` isn't guaranteed. See bean `gosd-85pt`.

Same artifact-release consequence as the Radxa Zero 3E's patch: this is a
kernel-pipeline change, so `rk3528-nanopi-zero2.dtb` needs rebuilding and
re-releasing before a real (non-`--artifacts-dir`) build picks it up. This
board has no artifacts-pipeline job yet at all (see the package doc comment
in `internal/boards/nanopizero2/board.go`), so this is folded into that
same pending work rather than being a new, separate gap.

## Diff against the Radxa Zero 3E fragment, and why

This fragment was built by starting from
`build/boards/radxa-zero-3e/kernel/kernel-fragment.config` (the other
Rockchip board GoSD supports) and re-verifying every symbol against the
actual RK3528 device tree and driver source at the pinned kernel tag, rather
than assuming the RK3566 config carries over unchanged.

| Symbol | Radxa Zero 3E | NanoPi Zero2 | Why |
|---|---|---|---|
| `CONFIG_ARCH_ROCKCHIP` | yes | yes | Same arm64 Rockchip umbrella symbol; no per-SoC Kconfig bool exists on arm64. |
| `CONFIG_MMC_DW`, `CONFIG_MMC_DW_ROCKCHIP` | yes | yes | rk3528's `sdmmc`/`sdio*` nodes use the same `rockchip,rk3288-dw-mshc` fallback compatible as the Radxa board's dw_mmc node. |
| `CONFIG_MMC_SDHCI_OF_DWCMSHC` | yes | yes | rk3528's `sdhci` (eMMC) node uses the same `rockchip,rk3588-dwcmshc` fallback compatible. |
| `CONFIG_STMMAC_ETH`, `CONFIG_STMMAC_PLATFORM`, `CONFIG_DWMAC_ROCKCHIP` | yes | yes | Both boards use the Synopsys DWMAC core via the Rockchip glue driver; `dwmac-rk.c` has an explicit `rockchip,rk3528-gmac` entry. |
| `CONFIG_REALTEK_PHY` | yes | yes | Both boards' GbE PHY is an RTL8211F (per board vendor spec sheets; the DT itself uses a generic PHY compatible on both boards, so the driver binds by probed chip ID). |
| `CONFIG_MOTORCOMM_PHY` | yes | **no** | Radxa carries this for a board-revision variant with a YT8531 PHY. The NanoPi Zero2 has no such variant — cut to keep the trim tight. |
| `CONFIG_USB_DWC3`, `CONFIG_USB_DWC3_DUAL_ROLE`, `CONFIG_PHY_ROCKCHIP_INNO_USB2`, `CONFIG_PHY_ROCKCHIP_NANENG_COMBO_PHY`, `CONFIG_USB_GADGET`, `CONFIG_USB_LIBCOMPOSITE`, `CONFIG_USB_CONFIGFS*` | yes (required + asserted) | **not required, not asserted** | Bean gosd-vcae's research (completed as part of this task) found mainline has **no RK3528 USB host/OTG controller DT node at all** as of the pinned tag — checked `rk3528.dtsi` and every RK3528 board file in the same kernel tree, plus `phy-rockchip-inno-usb2.c`'s of_device_id table (no rk3528 entry either). These symbols still end up `=y` in the generated config because the shared `arm64 defconfig` baseline enables them anyway (it does on Radxa too — that fragment's USB block mostly restates defconfig), but on this board they bind to nothing at runtime, so this fragment neither requires them nor asserts them in `internal/kernelspec.go`'s `RequiredY`. Recheck at a future kernel tag. |
| `CONFIG_GPIO_ROCKCHIP`, `CONFIG_I2C_RK3X`, `CONFIG_SPI_ROCKCHIP`, `CONFIG_SERIAL_8250_DW` | yes | yes | Same driver families; rk3528's i2c/spi/gpio/uart nodes use fallback compatibles (`rockchip,rk3399-i2c`, `rockchip,rk3066-spi`, `rockchip,gpio-bank`, `snps,dw-apb-uart`) that these same drivers already match. |

Effective-config check: diffing this board's generated `kernel.config`
against the Radxa Zero 3E's committed one, the **only** option-line
difference is `CONFIG_MOTORCOMM_PHY` (plus the compiler version string) —
everything else, including the defconfig-inherited USB stack, is identical
between the two boards' full configs.

## Known limitations

This has been build-tested but **not boot-tested on hardware** — that is
tracked separately in the bring-up task (gosd-odp7), which is itself blocked
on the board profile (gosd-wskc) and U-Boot (gosd-f39b). Do not treat a
clean build as proof the board boots.

USB (host or gadget) is not usable on this board at the pinned kernel tag —
see the diff table above. Per bean gosd-vcae's findings, the RK3528 dwc3
controller node and this board's USB-enable commit are already merged on
Linus's master but are absent from every numbered release so far, so USB
arrives with a future fleet-wide `KERNEL_TAG` bump rather than any work in
this pipeline. This does not block the epic's Ethernet-first scope, but does
defer USB-gadget-mode peripheral work (gosd-jge2 / gosd-uo9f / gosd-30jz)
on this board until then.

## Updating the pinned kernel version

Bump `fleetKernelTag` in `internal/kernelspec.go` to match the other boards'
pipelines (keep all boards on the same tag), rerun the build command above,
then copy `out/kernel.config` over the committed `kernel.config` and commit
both alongside the version bump. Re-check the USB gap above against the new
tag while you're at it.
