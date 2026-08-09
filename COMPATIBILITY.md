# Board / feature compatibility

GoSD turns a Go main package into flashable SD-card images for small ARM
boards. Most of what it does is board-independent — on every board, GoSD:

- Builds on macOS or Linux with no Docker, root, or cross-toolchain;
  kernels and bootloaders come from pinned, published artifacts.
- Provisions end users through Raspberry Pi Imager's WiFi/hostname wizard,
  via a [custom-repository catalog entry](docs/publishing.md); non-Pi
  boards appear under Imager's "No filtering" device option.
- Falls back to hand-editing [`gosd.toml`](docs/provisioning-formats.md)
  on the boot partition, so any flasher works.
- Passes configuration to the app as environment variables
  (`gosd.toml [env]`).
- Announces over mDNS — end users reach the device at `<hostname>.local`.
- Syncs time over SNTP (these boards have no battery-backed clock).
- Offers a [persistent `/data` partition](docs/runtime.md) that survives
  reflashes (`--data-size`), FAT32 by default with an opt-in ext4
  (`gosd build --data-filesystem=ext4`; unavailable on the Pi family — see
  the compatibility table).
- [Formats and mounts attached disks](docs/runtime.md) — USB drives, card
  readers, NVMe — from app code (the `disk` package); filesystem support
  varies by board (table below).
- Can [bundle prebuilt static Linux binaries](docs/externals.md) beside
  the app (`--with-external`).
- Can [expose an app to the internet with zero app code](docs/ingress.md)
  via a Cloudflare Tunnel (`--ingress cloudflared`; arm64 boards only) or a
  Tailscale Funnel (`--ingress tailscale-funnel`; every board, needs a
  `--data-size` data partition).
- Reserves placeholder files for
  [post-build config injection](docs/image-injection.md) (`--placeholder`).
- Can [compile a custom kernel](docs/custom-kernels.md) for drivers the
  stock kernels omit (`gosd build-kernel`).
- Enables I2C, SPI and GPIO by default ([pinouts](docs/runtime.md)) —
  except the Radxa Cubie A5E, see Board notes below.
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
| Radxa Cubie A5E | Not started — code-complete, artifacts published (bean `gosd-6pfn`) |

## Board-specific features

| Feature | Pi Zero 2W | Pi Zero W | Pi 3B | Radxa Zero 3E | NanoPi Zero2 | ROCK 4SE | Cubie A5E |
|---|---|---|---|---|---|---|---|
| Ethernet | ➖ | ➖ | ✅ | ✅ | ✅ | ✅ | ✅ [^cubie-eth] |
| WiFi (WPA2-PSK / open; hidden SSIDs) | ✅ | ✅ | ✅ [^pi3b-wifi] | ➖ | ➖ [^m2-wifi] | ❌ [^rock4se-wifi] | ❌ [^cubie-wifi] |
| USB gadget [^gadget-eth] | ✅ | ✅ | ➖ [^pi3b-gadget] | ✅ | ❌ [^nanopi-usb] | ✅ | ✅ |
| Onboard eMMC (`emmc` package) | ➖ | ➖ | ➖ | ✅ [^emmc-optional] | ✅ | ✅ [^emmc-optional] | ❌ [^cubie-emmc] |
| ext4 on the eMMC (the default) [^emmc-ext4] | ➖ | ➖ | ➖ | ✅ | ✅ | ✅ | ❌ [^cubie-emmc] |
| exFAT on the eMMC | ➖ | ➖ | ➖ | ✅ | ✅ | ✅ | ❌ [^cubie-emmc] |
| NVMe SSD (M.2) | ➖ | ➖ | ➖ | ➖ | ➖ | ✅ | ❌ [^cubie-nvme] |
| ext4 on attached disks (the default) | ✅ [^pi-ext4] | ✅ [^pi-ext4] | ✅ [^pi-ext4] | ✅ | ✅ | ✅ | ✅ |
| exFAT on attached disks | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| ext4 data partition (`gosd build --data-filesystem=ext4`; FAT32 is the default) | ❌ [^pi-data-ext4] | ❌ [^pi-data-ext4] | ❌ [^pi-data-ext4] | ✅ | ✅ | ✅ | ✅ |
| [Audio out](docs/sound.md) (via `gosd build-kernel`) | ✅ | ✅ | ✅ | 🚧 [^zero3e-audio] | ➖ | ✅ | ❌ [^cubie-audio] |
| [Ingress: Cloudflare Tunnel](docs/ingress.md) (`--ingress cloudflared`) | ✅ [^cloudflared-bench] | ❌ [^cloudflared-armv6] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] | ✅ [^cloudflared-bench] |
| [Ingress: Tailscale Funnel](docs/ingress.md) (`--ingress tailscale-funnel`) | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] | ✅ [^tsfunnel-bench] |

**Legend:** ✅ supported · 🚧 in progress · ➖ no such hardware on this
board · ❌ not supported (see footnote).

## Board notes

- **Pi Zero W** — single armv6 core (`GOARCH=arm GOARM=6`): a real
  performance ceiling for CPU-bound work.
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
  post-bring-up follow-up (bean `gosd-jpc8`). GPIO works as usual.

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

[^pi-data-ext4]: The stock Pi kernels don't build `CONFIG_EXT4_FS` in (same
    fact as `internal/blockmount`'s `remedyFor`) — the data partition stays
    FAT32-only on these boards; `gosd build --data-filesystem=ext4` refuses
    at build time rather than shipping an image whose data partition can
    never mount (bean `gosd-95yu`).

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

[^tsfunnel-bench]: Code-complete, unit- and QEMU-tested; not yet
    hardware-verified against a real Tailscale Funnel (epic `gosd-65uy`'s
    bench bean `gosd-79v8`). Unlike cloudflared, the shim is compiled by
    gosd itself for every board's architecture (including pi-zero-w's
    GOARM=6), so there is no upstream-asset gap here — `--data-size` (or
    `--data-size=expand`) is required so the shim's tailnet identity
    survives a reboot.

---

*An internal-only `qemu-virt` profile also exists for CI and local
testing; it is excluded from default builds and from this table.*
