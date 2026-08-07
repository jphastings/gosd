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
  reflashes (`--data-size`).
- [Formats and mounts attached disks](docs/runtime.md) — USB drives, card
  readers, NVMe — from app code (the `disk` package); filesystem support
  varies by board (table below).
- Can [bundle prebuilt static Linux binaries](docs/externals.md) beside
  the app (`--with-external`).
- Reserves placeholder files for
  [post-build config injection](docs/image-injection.md) (`--placeholder`).
- Can [compile a custom kernel](docs/custom-kernels.md) for drivers the
  stock kernels omit (`gosd build-kernel`).
- Enables I2C, SPI and GPIO by default ([pinouts](docs/runtime.md)).
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
| Radxa Cubie A5E | Not started — internal-only profile, no artifacts yet (epic `gosd-h1wv`) |

## Board-specific features

| Feature | Pi Zero 2W | Pi Zero W | Pi 3B | Radxa Zero 3E | NanoPi Zero2 | ROCK 4SE |
|---|---|---|---|---|---|---|
| Ethernet | ➖ | ➖ | ✅ | ✅ | ✅ | ✅ |
| WiFi (WPA2-PSK / open; hidden SSIDs) | ✅ | ✅ | ✅ [^pi3b-wifi] | ➖ | ➖ [^m2-wifi] | ❌ [^rock4se-wifi] |
| USB gadget [^gadget-eth] | ✅ | ✅ | ➖ [^pi3b-gadget] | ✅ | ❌ [^nanopi-usb] | ✅ |
| Onboard eMMC (`emmc` package) | ➖ | ➖ | ➖ | ✅ [^emmc-optional] | ✅ | ✅ [^emmc-optional] |
| NVMe SSD (M.2) | ➖ | ➖ | ➖ | ➖ | ➖ | ✅ |
| ext4 on attached disks (the default) | ❌ [^pi-ext4] | ❌ [^pi-ext4] | ❌ [^pi-ext4] | ✅ | ✅ | ✅ |
| exFAT on attached disks | ✅ | ✅ | ✅ | 🚧 [^exfat-soon] | 🚧 [^exfat-soon] | ✅ |
| [Audio out](docs/sound.md) (via `gosd build-kernel`) | ✅ | ✅ | ✅ | 🚧 [^zero3e-audio] | ➖ | ✅ |

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

[^pi-ext4]: The Pi kernels lack `CONFIG_EXT4_FS`; asking for ext4 —
    including via the default — fails fast with `disk.ErrUnsupportedFS`.
    Use FAT32 or exFAT here.

[^exfat-soon]: Enabled in these boards' kernel fragments but not yet in a
    published artifacts release; until then exFAT fails fast with
    `disk.ErrUnsupportedFS`.

[^zero3e-audio]: Recipe written, never compiled or heard (bean
    `gosd-lrxz`).

---

*An internal-only `qemu-virt` profile also exists for CI and local
testing; it is excluded from default builds and from this table.*
