# Board / feature compatibility

GoSD turns a Go main package into flashable SD-card images for small ARM
boards. Most of what it does is board-independent — on every board, GoSD:

- Builds on macOS or Linux with no Docker, root, or cross-toolchain;
  kernels and bootloaders come from pinned, published artifacts.
- Provisions end users through Raspberry Pi Imager's WiFi/hostname wizard,
  via a [custom-repository catalog entry](docs/publishing.md); non-Pi
  boards appear under Imager's "No filtering" device option.
- Falls back to hand-editing [the config tree](docs/config.md)
  on the boot partition, so any flasher works.
- Passes configuration to the app as environment variables
  (`config/env/`).
- Announces over mDNS — end users reach the device at `<hostname>.local`.
- Syncs time over SNTP (these boards have no battery-backed clock).
- Offers a [persistent `/data` partition](docs/runtime.md) that survives
  reflashes (`--data-size`), FAT32 by default with an opt-in ext4
  (`gosd build --data-filesystem=ext4` — see the compatibility table).
- [Formats and mounts attached disks](docs/runtime.md) — USB drives, card
  readers, NVMe — from app code (the `disk` package); filesystem support
  varies by board (table below).
- Can [bundle prebuilt static Linux binaries](docs/externals.md) beside
  the app (`--with-external`).
- Can [expose an app to the internet with zero app code](docs/ingress.md)
  via a Cloudflare Tunnel (`--ingress cloudflared`; arm64 boards only) or a
  Tailscale Funnel (`--ingress tailscale-funnel`; every board, needs a
  `--data-size` data partition).
- Publishes every setting on the card, and any reserved placeholder file, for
  [post-build config injection](docs/image-injection.md) (`--config-dir`,
  `--placeholder`).
- Can [compile a custom kernel](docs/custom-kernels.md) for drivers the
  stock kernels omit (`gosd build-kernel`).
- Enables I2C, SPI and GPIO by default ([pinouts](docs/runtime.md)) —
  except the Radxa Cubie A5E, and except SPI on the Pi Zero W pending an
  artifact release, see Board notes below.
- Shows boot state on an onboard status LED — even flash while booting, a
  short blip once a second while your app runs, solid on for a recorded
  fatal error — no
  code changes, no config: [which LED gets used and
  why](docs/status-led.md). Code-complete and unit-tested; not yet
  bench-verified on any board (bean `gosd-xtcs`).
- Ships no shell and no SSH, ever — serial console and app logs only.
- Will gain OTA app updates — [designed](docs/design/ab-updates.md), not
  yet implemented (epic `gosd-vxal`).

Below is a snapshot of `main`, not a roadmap (`beans list` shows what's in
flight). ✅ means **code-complete**: implemented, unit-tested, QEMU-tested
where applicable. Hardware verification happens per feature during board
bring-ups and is recorded in the beans named here; a completed bring-up
proves the common core on real hardware (build → flash → boot → serial
console → network up → mDNS + HTTP → power-cycle survival).

## Bring-up status

| Board | Bring-up |
|---|---|
| Raspberry Pi Zero 2W | Complete (`gosd-m9dj`) |
| Raspberry Pi Zero W | Complete (`gosd-qltr`) |
| Raspberry Pi 3B | Maiden boot proven; checklist open (`gosd-f5xm`) |
| Radxa Zero 3E | In progress (`gosd-nlzf`) |
| NanoPi Zero2 | Complete (`gosd-odp7`) |
| Radxa ROCK 4SE | Complete (`gosd-sz6p`) |
| Radxa Cubie A5E | Boot proven on the 1GB variant; 2GB/4GB unverified — see board notes (bean `gosd-6pfn`) |

## Board-specific features

| Feature | Pi Zero 2W | Pi Zero W | Pi 3B | Radxa Zero 3E | NanoPi Zero2 | ROCK 4SE | Cubie A5E |
|---|---|---|---|---|---|---|---|
| Ethernet | ➖ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ [^cubie-eth] |
| WiFi (WPA2-PSK / open; hidden SSIDs) | ✅ | ✅ | ✅ [^pi3b-wifi] | ➖ | ➖ [^m2-wifi] | ❌ [^rock4se-wifi] | ❌ [^cubie-wifi] |
| USB gadget [^gadget-eth] | ✅ | ✅ | ➖ [^pi3b-gadget] | ✅ | ❌ [^nanopi-usb] | ✅ | ✅ [^cubie-gadget] |
| Onboard eMMC (`emmc` package) | ➖ | ➖ | ➖ | ✅ [^emmc-optional] | ✅ | ✅ [^emmc-optional] | ❌ [^cubie-emmc] |
| ext4 on the eMMC (the default) [^emmc-ext4] | ➖ | ➖ | ➖ | ✅ | ✅ | ✅ | ❌ [^cubie-emmc] |
| exFAT on the eMMC | ➖ | ➖ | ➖ | ✅ | ✅ | ✅ | ❌ [^cubie-emmc] |
| NVMe SSD (M.2) | ➖ | ➖ | ➖ | ➖ | ➖ | ✅ | ❌ [^cubie-nvme] |
| ext4 on attached disks (the default) | ✅ [^pi-ext4] | ✅ [^pi-ext4] | ✅ [^pi-ext4] | ✅ | ✅ | ✅ | ✅ |
| exFAT on attached disks | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| ext4 data partition (`gosd build --data-filesystem=ext4`; FAT32 is the default) | ✅ [^pi-data-ext4] | ✅ [^pi-data-ext4] | ✅ [^pi-data-ext4] | ✅ | ✅ | ✅ | ✅ |
| [Audio out](docs/sound.md) (via `gosd build-kernel`) | ✅ | ✅ | ✅ | 🚧 [^zero3e-audio] | ➖ | ✅ | ❌ [^cubie-audio] |
| [Ingress: Cloudflare Tunnel](docs/ingress.md) (`--ingress cloudflared`) | ✅ [^cloudflared-bench] | ❌ [^cloudflared-armv6] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] |
| [Ingress: Tailscale Funnel](docs/ingress.md) (`--ingress tailscale-funnel`) | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] |

**Legend:** ✅ supported · 🚧 in progress · ➖ no such hardware on this
board · ❌ not supported (see footnote).

## Board notes

- **Pi Zero W** — single armv6 core (`GOARCH=arm GOARM=6`): a real
  performance ceiling for CPU-bound work. Its SPI controller needs a
  kernel-build DTS patch, not `config.txt`'s `dtparam=spi=on`: this is the
  one GoSD board built from the mainline-style DTS chain, which (unlike the
  downstream-style DTBs the other Pi boards use) has no `__overrides__`
  node for the Pi firmware's dtparam mechanism to patch, so that line is
  silently discarded (bean `gosd-dkqb`). The fix is in tree
  (`build/boards/pi-zero-w/kernel/patches/0003-enable-header-spi.patch`)
  but needs a new `artifacts/vX.Y.Z` release before a real (non-
  `--artifacts-dir`) build picks it up — until then, `/dev/spidev0.*` will
  not appear on this board. I2C is unaffected: `bcm2835-rpi.dtsi` enables
  it unconditionally, independent of the (also inert on this board)
  `dtparam=i2c_arm=on`.
- **Pi 3B** — one image covers the 3B and 3B+ (both DTBs ship; the
  firmware picks by board revision).
- **Radxa Zero 3E** — its [1,500,000-baud console](docs/runtime.md)
  garbles on CP210x/PL2303 USB-serial adapters; use a CH340 adapter or
  `gosd build --console-baud 115200`.
- **NanoPi Zero2** — GPIO/I2C/SPI are on a 30-pin FPC connector, not a
  Pi-style 40-pin header.
- **Radxa Cubie A5E** — the first Allwinner board: unlike every other
  board here, its stock kernel has no header SPI at all
  (`sun55i-a523.dtsi` has no SPI node at this kernel tag, so there's
  nothing for a DTS patch to enable) and no header I2C either — only the
  PMIC-internal `r_i2c0` bus is wired up; both are deferred to a
  post-bring-up follow-up (bean `gosd-jpc8`). GPIO works as usual. Its
  U-Boot ships per-chip vendor DRAM calibration values hardware-verified
  only on the **1GB LPDDR4x variant** (bean `gosd-6pfn`) — upstream
  mainline's own values fail U-Boot SPL's DRAM init on that unit
  (`DRAM test failure at address 0x6fffffc0`, bean `gosd-84b8`). The
  2GB/4GB variants have not been tested and may fail to boot with the same
  DRAM error at a different address; if you're running one, feedback
  (working or not) is very welcome on bean `gosd-84b8`.

[^pi3b-wifi]: The 3B+'s BCM43455 WiFi firmware isn't bundled yet, so a
    3B+ is Ethernet-first for now (bean `gosd-oq0z`).

[^m2-wifi]: WiFi needs an optional M.2 module; out of scope for now
    (epic `gosd-cwjf`).

[^rock4se-wifi]: The onboard WiFi/BT has no driver in the stock kernel —
    out of scope for this board's wired use case (epic `gosd-cuym`).

[^gadget-eth]: CDC-ACM serial and mass storage today; USB-Ethernet
    (ECM/RNDIS) is not yet built (bean `gosd-30jz`).

[^pi3b-gadget]: The SoC's only USB port is hard-wired through the onboard
    hub/Ethernet chip, so peripheral mode is physically impossible;
    `--usb-gadget` fails fast for this board.

[^nanopi-usb]: The RK3528's USB device-tree nodes aren't in any released
    mainline kernel yet, so this board has no USB at all — host or gadget
    (bean `gosd-36yy`); `--usb-gadget` refuses to build.

[^cubie-gadget]: Building with `--usb-gadget` ships a variant device tree
    with the USB-C port's `ehci0`/`ohci0` host controllers disabled, because
    they share a phy with the peripheral controller and this board has no
    ID/VBUS detection to arbitrate — without it the host side wins at probe
    and the port can never enumerate as a device, whatever `dr_mode` says
    (bench-proven, bean `gosd-3io0`). **The trade is that a gadget-mode image
    cannot use the USB-C port as a host**; the USB 3.0 Type-A port is
    unaffected. Bench-verified end to end: the board enumerates on a host as
    a CDC-ACM device and echoes lines back over it.

[^emmc-optional]: eMMC is an optional module/SKU on these boards; with
    none fitted, `emmc.FormatAndMount` returns `ErrNoEMMC`.

[^emmc-ext4]: `emmc.Options.Filesystem`'s zero value (bean `gosd-9sc4`,
    mirroring `disk`'s default, epic `gosd-lfu0`) — every board with a
    working onboard eMMC already has `CONFIG_EXT4_FS` (same Rockchip
    arm64 defconfig inheritance as "ext4 on attached disks" below), so
    there is no eMMC-having board where the default fails.

[^pi-ext4]: Shipped in artifacts v0.10.0 (bean `gosd-19kw`); the
    `/proc/filesystems` preflight passes from that release on. Not yet
    exercised on Pi hardware — an on-device spot-check rides the next
    bench pass.

[^pi-data-ext4]: `CONFIG_EXT4_FS` has been built into the stock Pi kernels
    since artifacts v0.10.0 (bean `gosd-19kw`; see [^pi-ext4]), so
    `gosd build --data-filesystem=ext4` (bean `gosd-95yu`) no longer refuses
    for these boards. Verification differs per board: Pi Zero 2W has
    actually been bench-booted with an ext4 data partition — format, grow,
    mount, re-adoption after reflash, and a hard-power-cut crash-durability
    check all proven (bean `gosd-7bwv`). Pi 3B shares the Zero 2W's arm64
    kernel pin and is enabled on that same released-kernel evidence, but
    hasn't had its own bench pass yet. Pi Zero W is the fleet's only
    32-bit board and has never run ext4 on real hardware at all.

[^zero3e-audio]: Recipe written, never compiled or heard (bean
    `gosd-lrxz`).

[^cubie-eth]: Only EMAC0 (RGMII, RTL8211F PHY, `dwmac-sun8i`) is enabled —
    the board's second GbE port uses the newer GMAC200 IP, which has no
    driver at this kernel tag (epic `gosd-h1wv`).

[^cubie-wifi]: The onboard WiFi 6/BT 5.4 module has no mainline driver at
    this kernel tag — out of scope for this board's initial support
    (epic `gosd-h1wv`).

[^cubie-emmc]: The board has an optional onboard eMMC module socket, but
    no eMMC node is enabled in the board DT at this kernel tag (bean
    `gosd-jpc8`'s research) — `emmc.FormatAndMount` returns `ErrNoEMMC`,
    the same as a board with no eMMC hardware at all.

[^cubie-nvme]: The M.2 Key-M slot (PCIe Gen2 x1, shared combo PHY with
    USB3) has no PCIe controller node in the board DT at this kernel tag —
    mainline A523 PCIe support isn't there yet (epic `gosd-h1wv`).

[^cubie-audio]: The stock kernel disables `CONFIG_SOUND` entirely; no
    recipe has been written for this board yet (no bean filed).

[^cloudflared-bench]: Code-complete, unit- and QEMU-tested; not yet
    hardware-verified against a real Cloudflare Tunnel (bench bean
    `gosd-igk0`).

[^cloudflared-armv6]: cloudflared's official `arm` release is built for
    GOARM=7 and faults with "illegal instruction" on this board's armv6
    CPU; `gosd build --ingress cloudflared` refuses to build for it
    (revisit if upstream ever ships a GOARM=6 build). See
    `docs/ingress.md`.

[^tsfunnel-bench]: Hardware-verified end to end on `nanopi-zero2` — the shim
    registers on the tailnet and serves the app over a public
    `https://<host>.<tailnet>.ts.net` Funnel URL (epic `gosd-65uy`'s bench
    bean `gosd-79v8`); the other boards run the identical, gosd-compiled shim
    and share the same runtime path (QEMU-tested). This works because the
    shim ships full tsnet: a build-tag feature trim silently broke tsnet's
    control-plane registration (bean `gosd-h46e`). Unlike cloudflared, the
    shim is compiled by gosd itself for every board's architecture (including
    pi-zero-w's GOARM=6), so there is no upstream-asset gap here —
    `--data-size` (or `--data-size=expand`) is required so the shim's tailnet
    identity survives a reboot.

---

*An internal-only `qemu-virt` profile also exists for CI and local
testing; it is excluded from default builds and from this table.*
