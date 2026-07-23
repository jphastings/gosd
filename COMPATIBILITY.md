# Board / feature compatibility

What works on which board, as of the current `main`. This is a snapshot of
repo state (code, kernel configs, and tracked work in beans), not a roadmap —
see `beans list` for what's in flight.

> **Only one board has been through hardware bring-up so far: the Radxa
> ROCK 4SE** (bean `gosd-sz6p`, closed 2026-07-23). Every other ✅ below
> means *code-complete: implemented, unit-tested, and (where applicable)
> QEMU-tested via the internal `qemu-virt` profile* — not "verified on a real
> device." Hardware bring-up for the Pi Zero 2W and Radxa Zero 3E is tracked
> by beans `gosd-m9dj` and `gosd-nlzf`; until those close, treat every ✅ as
> "should work" rather than "confirmed working." The Pi Zero W is the same:
> code-complete and fake-artifact-tested (bean `gosd-et0q`), no bring-up bean
> filed yet. The NanoPi Zero2 is the same again: code-complete and
> fake-artifact-tested (bean `gosd-wskc`), with hardware bring-up tracked by
> bean `gosd-odp7`. The Radxa ROCK 4SE's own bring-up only exercised a subset
> of this table's rows (see the footnotes below for exactly which); every
> ROCK 4SE row without a bring-up-dated footnote is still code-complete-only,
> the same as every other board.

| Feature | Raspberry Pi Zero 2W | Raspberry Pi Zero W | Radxa Zero 3E | NanoPi Zero2 | Radxa ROCK 4SE |
|---|---|---|---|---|---|
| Image build via `gosd build` | ✅ | ✅ [^armv6-perf] | ✅ | ✅ | ✅ |
| Published artifacts (kernel/bootloader) | ✅ | ✅ | ✅ | ✅ [^nanopi-artifacts] | ✅ [^rock4se-artifacts] |
| Custom kernel (`gosd build-kernel`) | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] |
| Bundle prebuilt static binary (`--with-external`) | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] |
| Ethernet | ➖ [^pi-no-eth] | ➖ [^pi-no-eth] | ✅ | ✅ | ✅ |
| WiFi (WPA2-PSK / open) | ✅ | ✅ [^pi-zero-w-wifi] | ➖ [^no-radio] | ➖ [^nanopi-wifi] | ❌ [^rock4se-wifi] |
| Hidden-SSID WiFi | ✅ [^hidden-ssid] | ✅ [^hidden-ssid] | ➖ [^no-radio] | ➖ [^nanopi-wifi] | ❌ [^rock4se-wifi] |
| Imager catalog provisioning | ✅ [^pi-tag] | ✅ [^pi-zero-w-tag] | ✅ [^no-filtering] | ✅ [^no-filtering] | ✅ [^no-filtering] |
| `gosd.toml` config (fallback) | ✅ | ✅ | ✅ | ✅ | ✅ |
| App env vars (`gosd.toml [env]`) | ✅ | ✅ | ✅ | ✅ | ✅ |
| mDNS (`<hostname>.local`) | ✅ | ✅ | ✅ | ✅ | ✅ |
| SNTP time sync | ✅ | ✅ | ✅ | ✅ | ✅ |
| Persistent `/data` partition | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] |
| Onboard eMMC format/mount (`emmc` package) | ➖ [^no-emmc] | ➖ [^no-emmc] | ✅ [^emmc] | ✅ [^emmc] | ✅ [^emmc][^rock4se-emmc] |
| USB gadget (serial/Ethernet/mass storage) | ✅ [^usb-gadget] | ✅ [^usb-gadget] | ✅ [^usb-gadget] | ❌ [^nanopi-usb] | ✅ [^usb-gadget][^rock4se-otg] |
| NVMe SSD (M.2) + exFAT | ➖ [^no-m2] | ➖ [^no-m2] | ➖ [^no-m2] | ➖ [^no-m2] | ✅ [^rock4se-nvme] |
| I2C | ✅ [^i2c] | ✅ [^i2c] | ✅ [^i2c] | ✅ [^i2c][^nanopi-fpc] | ✅ [^i2c] |
| GPIO | ✅ [^gpio] | ✅ [^gpio] | ✅ [^gpio] | ✅ [^gpio][^nanopi-fpc] | ✅ [^gpio] |
| SPI | ✅ [^spi] | ✅ [^spi] | ✅ [^spi] | ✅ [^spi][^nanopi-fpc] | ✅ [^spi] |
| OTA app updates | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] |

**Legend:** ✅ implemented · 🚧 planned or in-progress · ➖ not applicable
(no matching hardware) · ❌ not supported (with a reason below).

## Footnotes

[^custom-kernel]: `gosd build-kernel` (see `docs/custom-kernels.md`) is
    code-complete: it drives a local Docker/Podman daemon to cross-compile a
    board's kernel from `internal/kernelspec`'s declarative build inputs
    plus a developer's `gosd-kernel.toml` overlay, emitting artifacts that
    drop straight into `gosd build --artifacts-dir`. The command's pipeline
    was verified end-to-end on the internal `qemu-virt` profile: a real
    Docker build fed straight into a qemu boot-to-HTTP run. Per-board custom
    kernels (this row) are fake-artifact/CI-tested for every public
    board; the flagship worked example — compiling in USB DVB-T support on
    the Pi Zero 2W, including the documented rp1-cfe collision workaround —
    was additionally proven with a real Docker build producing a
    `kernel.config` with every expected symbol `=y`. Like every other row in
    this table, no custom kernel has been run on physical hardware yet
    (hardware bring-up beans for the underlying boards are still open, see
    the note above this table).

[^with-external]: `gosd build --with-external <path>[:<dest>]` (repeatable)
    bundles a prebuilt, fully static executable into the initramfs at mode
    0755 (see `docs/runtime.md#bundling-a-companion-binary---with-external`).
    Build-time validation (`debug/elf`) checks each binary's ELF class/
    machine against the board's architecture and rejects a dynamically
    linked binary (`PT_INTERP` present) with an actionable error, so this
    row is code-complete and fake-artifact-tested against real
    cross-compiled static Go binaries for both arm64 and armv6 boards; like
    every other row, it hasn't been exercised on physical hardware yet. The
    binary itself doesn't have to be Go: `gosd build-external` (see
    `docs/externals.md`) cross-compiles one from a `gosd-external.toml`
    recipe inside Docker/Podman, arch-keyed rather than per-board (an
    arm64 build covers every board except the armv6 pi-zero-w alike), so it
    isn't its own row in this per-board table.

[^nanopi-artifacts]: The NanoPi Zero2's kernel and U-Boot are both built and
    published by CI (`nanopi-zero2-kernel` and `nanopi-zero2-uboot` jobs,
    `.github/workflows/build-artifacts.yml`). U-Boot is pinned to
    **`v2026.07-rc5`** rather than a final release: mainline U-Boot's
    dedicated `nanopi-zero2-rk3528_defconfig` is new in the v2026.07 cycle
    and wasn't in any prior tagged release, and JP asked to pin the latest
    rc now rather than wait for the final tag so this board is
    hardware-testable sooner (bean `gosd-f39b`'s amended gate). Re-pinning to
    the final `v2026.07` release once it ships is an open item on that bean.

[^rock4se-artifacts]: The ROCK 4SE's kernel and U-Boot are built and
    published by CI (`rock-4se-kernel` and `rock-4se-uboot` jobs) from the
    `artifacts/v0.5.0` release onward. Its U-Boot is the first **blob-free
    Rockchip bootloader** in GoSD: RK3399's DRAM init is open-source in
    U-Boot's TPL and BL31 is compiled from mainline Trusted-Firmware-A
    (pinned in `build/boards/rock-4se/manifest.json`) — no rkbin fetch at
    all, unlike the RK3566/RK3528 boards. See
    `build/boards/rock-4se/uboot/README.md`.

[^rock4se-wifi]: The ROCK 4SE has onboard WiFi/BT hardware (and its DT
    nodes exist at the pinned kernel tag), but the stock kernel ships no
    driver for it: onboard WiFi/BT is explicitly out of scope for the
    board's initial support (epic `gosd-cuym` locked decision — this board
    joined GoSD for a wired/NVMe appliance use case). A follow-up bean can
    enable it later; until then this board is Ethernet-first, like the
    other Rockchip boards.

[^rock4se-emmc]: The ROCK 4SE's eMMC is an optional plug-in module
    (socket), not soldered storage. The RK3399's Arasan eMMC controller
    driver (`CONFIG_MMC_SDHCI_OF_ARASAN`) is present in the published
    stock `kernel.config`, but only incidentally from the defconfig
    baseline — no fragment or `RequiredY` asserts it. With no module
    fitted, `emmc.FormatAndMount` returns `ErrNoEMMC` as on the Pi boards —
    **hardware-confirmed** during bring-up (bean `gosd-sz6p`, 2026-07-23)
    via `examples/usbwebsite`'s graceful no-eMMC degradation. The actual
    format-and-mount codepath needs a fitted module to exercise and remains
    code-complete-only on this board, same as the rest of the table (see
    [^emmc]).

[^rock4se-otg]: The stock kernel's DTS patch flips `usbdrd_dwc3_0` to
    `dr_mode = "peripheral"` for gadget mode. This was a **best guess** at
    which of the RK3399's two dwc3 controllers is wired to the board's
    physical host/device-switch OTG port (the shared upstream DTS treats
    both symmetrically; gosd-je2r couldn't resolve it from DTS text) —
    **hardware-verified correct** during bring-up (bean `gosd-sz6p`,
    2026-07-23): a CDC-ACM gadget (`examples/usbserial`) enumerated on the
    host and echoed data end to end over `/dev/ttyGS0`, confirming
    `usbdrd_dwc3_0` (`0xfe800000`, the board's top blue USB 3.0 port,
    furthest from the Ethernet jack) is the right controller with no swap
    to `usbdrd_dwc3_1` needed.

[^no-m2]: No NVMe-capable M.2 slot on this board — a hardware limitation,
    not a GoSD gap. (The NanoPi Zero2's M.2 Key-E socket is for WiFi
    modules, not NVMe storage.)

[^rock4se-nvme]: The ROCK 4SE's stock kernel enables the RK3399 PCIe host,
    its PHY, the NVMe block driver, and the exFAT filesystem (+UTF-8 NLS),
    all asserted by the board's kernel fragment — so an M.2 NVMe SSD
    formatted exFAT (host-native for USB mass-storage sharing) is mountable
    by an app via `unix.Mount`. **Hardware-verified** during bring-up (bean
    `gosd-sz6p`, 2026-07-23) with the actual target SSD (KIOXIA
    XG7000-512): the previously flagged RK3399 PCIe link-training risk
    didn't manifest — the drive enumerated immediately, sustained 256 MiB
    @ 840 MB/s sequential read, and exFAT mounted via `unix.Mount` with
    data surviving unmount/remount. (The link-training timeout logged with
    the slot empty was confirmed benign probing noise, not a real fault.)

[^pi-no-eth]: Neither the Raspberry Pi Zero 2 W nor the original Zero W has
    an onboard Ethernet port (WiFi only) — this is a hardware limitation of
    both boards, not a GoSD gap. `gosd-init`'s wired-networking code
    (`cmd/gosd-init/internal/netup`) matches any `eth*`/`end*`/`enp*`
    interface generically, so a USB-Ethernet adapter on the micro-USB OTG
    port would likely work through the same DHCP path, but this is untested
    and not a documented/supported configuration.

[^armv6-perf]: The Zero W's BCM2835 has a single ARM1176JZF-S core at armv6
    (`GOARCH=arm GOARM=6`, no NEON) — a fraction of the Zero 2 W's quad-core
    64-bit Cortex-A53. Both the app and gosd-init are cross-compiled for this
    target (see the "Target" locked decision in `CLAUDE.md` and bean
    `gosd-2j6z`'s per-arch build pipeline), so this is a real, expected
    performance ceiling for any CPU-bound app logic on this board, not a
    missing optimization — plan accordingly for anything heavier than
    GPIO/network I/O.

[^pi-zero-w-wifi]: The Zero W's WiFi/BT combo chip is a single revision,
    plain BCM43430 (unlike the Zero 2 W's three chip revisions) — the
    board's kernel enables `CFG80211`/`BRCMFMAC`, and its board profile
    (`internal/boards/pizerow`) ships the matching firmware blob (fetched
    from upstream's Cypress-branded `cyfmac43430-sdio.*`, per bean
    `gosd-06kj`'s findings) plus its board-specific alias names, flattened
    into `/lib/firmware/brcm` the same way pi-zero-2w's are.

[^no-radio]: The Radxa Zero 3E has no WiFi radio — its kernel build carries
    no `cfg80211`/`brcmfmac`-equivalent driver, and its board profile
    (`internal/boards/radxazero3e`) declares no runtime-loaded firmware.
    Ethernet-only by hardware.

[^nanopi-wifi]: WiFi on the NanoPi Zero2 is only available via an optional
    M.2 Key-E module; no specific module has been chosen, and M.2 WiFi
    support is explicitly out of scope for now (epic `gosd-cwjf`). This board
    is Ethernet-first.

[^hidden-ssid]: `internal/provision` parses Imager's `hidden: true` flag onto
    a network's `Hidden` field, and `wifiup` now threads it through the
    credential chain and joins by issuing nl80211 CONNECT directly for the
    named SSID rather than requiring a prior scan match — the pinned
    `mdlayher/wifi` doesn't expose a directed-scan-by-SSID API, but
    brcmfmac's own join path already does an active/directed probe for the
    given SSID as part of association, so no scan step was needed either
    way (bean `gosd-lbpm`). Code-complete and fake-tested; pending bench
    verification against a real hidden test AP on the Pi Zero 2W.

[^pi-tag]: Raspberry Pi Imager has no device-specific tag for the Zero 2 W —
    it shares the "Raspberry Pi Zero 2 W" device's tags (`pi3-64bit`/
    `pi3-32bit`) with the Pi 3 family. Consequence: a GoSD Pi Zero 2W catalog
    entry also appears when a user selects **Raspberry Pi 3** in Imager's
    device-filter step, not only when they select the Zero 2 W. This is an
    Imager limitation (see `docs/publishing.md`), not a GoSD bug.

[^pi-zero-w-tag]: Raspberry Pi Imager's device-filter list has no
    Zero-W-specific tag either — its "Raspberry Pi Zero" device entry
    (description: "Raspberry Pi Zero, Zero W, and Zero WH") carries tags
    `["pi1-32bit"]`, fetched and inspected directly against
    `downloads.raspberrypi.org/os_list_imagingutility_v4.json` on 2026-07-07
    (see `internal/catalog.boardImagerDeviceTags`). GoSD's pi-zero-w image is
    armv6/32-bit, matching that tag exactly. Consequence, the same shape as
    the Pi Zero 2 W's tag sharing above: the same catalog's "Raspberry Pi 1"
    device entry also carries exactly `["pi1-32bit"]`, so a GoSD Pi Zero W
    catalog entry also appears when a user selects **Raspberry Pi 1** in
    Imager's device-filter step, not only when they select the Zero/Zero W.

[^no-filtering]: Raspberry Pi Imager's device-filter list contains only
    official Raspberry Pi hardware, so no non-Pi board (Radxa, NanoPi) can
    ever match a real device tag. GoSD's catalog entries for these boards
    carry the board ID as a deliberately non-matching tag, so they're
    correctly generated and schema-valid, but only become visible to an end
    user when they pick **No filtering** on Imager's device-selection page.
    See "Device filtering" in `docs/publishing.md`.

[^data-opt-in]: The `GOSD-DATA` partition is opt-in at build time —
    `gosd build --data-size` defaults to `0` (no partition; `/data` mounts
    read-only), so pass a size (e.g. `--data-size=1GiB`) to get writable
    persistence. The capability itself is unchanged and identical across all
    boards; see `docs/runtime.md`'s "Persistent storage: `/data`"
    section.

[^no-emmc]: Neither Raspberry Pi board has onboard eMMC — this is a hardware
    limitation of both boards, not a GoSD gap. The `emmc` package's
    `FormatAndMount` returns `ErrNoEMMC` on these boards.

[^emmc]: The `emmc` package (public API, see `docs/runtime.md`'s "Onboard
    eMMC" section) auto-discovers the board's onboard eMMC — distinguishing
    it from the booted microSD card, which is never a format target — and
    formats it with a whole-device FAT filesystem the first time it's seen
    blank, mounting-only on every run after that. It carries the same
    FAT-only caveats as the `/data` partition (no unix permissions/symlinks,
    not power-loss-robust; write with the temp-file+fsync+rename pattern).
    Same caveat as the rest of this table: code-complete and unit-tested, not
    yet hardware-verified. `examples/emmcstorage` is the worked example. On
    the ROCK 4SE specifically, the *no-module-fitted* branch (`ErrNoEMMC`) is
    hardware-confirmed (bean `gosd-sz6p`, 2026-07-23, see
    [^rock4se-emmc]) — the actual format/mount path is still
    code-complete-only on every board, this one included.

[^usb-gadget]: The kernel config for USB gadget mode (DWC2 on both Pi
    boards, DWC3 on the Radxa boards; `CONFIG_USB_GADGET`, configfs,
    ACM/ECM/RNDIS functions) is already enabled on every gadget-capable
    board's kernel. The pure-Go configfs
    gadget library (package `gadget`, a public v0.3 API surface) is
    implemented and unit-tested against a fake filesystem, with CDC-ACM
    serial gadget mode working end to end (`gosd build --usb-gadget`, see
    `examples/usbserial` and `docs/runtime.md`'s "USB gadget mode" section)
    — bean `gosd-uo9f`. USB Ethernet (ECM/RNDIS) isn't built yet (bean
    `gosd-30jz`). USB mass storage (`gadget.MassStorage`, configfs
    `f_mass_storage`: one LUN backed by a block device or disk-image file,
    with read-only and removable flags) is implemented and unit-tested the
    same way (bean `gosd-k2fs`). Mass storage additionally needs
    `CONFIG_USB_CONFIGFS_MASS_STORAGE=y` in the board kernel: every current
    board's recorded published `kernel.config` already carries it, but only
    incidentally — inherited from the defconfig baseline, asserted by no
    kernel fragment or `internal/kernelspec` `RequiredY` list — so the
    *guaranteed* enablement lands when the fragments gain it explicitly at
    the next fleet kernel tag bump (never a single-board bump). The
    exception is the Radxa ROCK 4SE (epic `gosd-cuym`), which asserts it in
    its stock kernel fragment and `RequiredY` from the start. Like every other ✅ in this table, this means code-complete
    and unit-tested, not hardware-verified on the Pi boards or the Radxa
    Zero 3E: no on-device USB enumeration has been tried on those boards
    yet, blocked on hardware bring-up (`gosd-m9dj`, `gosd-nlzf`), which are
    themselves blocked on acquiring a bring-up kit (`gosd-s4t4`).
    **Exception: the Radxa ROCK 4SE's CDC-ACM path is hardware-verified**
    (bean `gosd-sz6p`, 2026-07-23, see [^rock4se-otg]) — USB mass storage
    and every other board's gadget support remain unverified on real
    hardware.

[^nanopi-usb]: The RK3528 SoC has no USB controller DT node in any numbered
    mainline kernel release as of the pinned tag (v6.18.37) — the `dwc3` node
    and the board's USB-enable commit exist only on Linux's development
    `master`, not yet in a release. Confirmed directly against the pinned
    kernel source (bean `gosd-rqx8`). Consequence: the NanoPi Zero2 has no
    USB at all — host or gadget — until a future fleet-wide kernel version
    bump picks up that commit; Ethernet, SD/eMMC, and serial console are
    unaffected. Recheck when bumping the pinned kernel tag.

[^i2c]: I2C is enabled by default on every board as of bean `gosd-85pt` — no
    build flag needed, and there's no opt-out today. Mechanism differs by
    board family: the Pi boards gained `dtparam=i2c_arm=on` in `config.txt`;
    the Rockchip boards gained a kernel-build-time device-tree patch
    (`build/boards/radxa-zero-3e/kernel/patches/`,
    `build/boards/nanopi-zero2/kernel/patches/`,
    `build/boards/rock-4se/kernel/patches/`) enabling the header-routed
    `i2cN` controller node, since the pinned U-Boot on all three doesn't
    support `CONFIG_OF_LIBFDT_OVERLAY`/extlinux `fdtoverlays` (checked
    directly against all three defconfigs) — so this ✅ carries the same "code-complete,
    fake-artifact-tested, not hardware-verified" caveat as the rest of this
    table, plus one additional wrinkle: the Rockchip boards' DTB artifact
    needs a new artifacts release (tag bump) before a real, non-
    `--artifacts-dir` build picks up the change. Per-board bus and pin
    numbers are documented in `docs/runtime.md`'s "GPIO, I2C, SPI" section;
    `examples/i2cscan` is the worked, cross-board example. GPIO and SPI are
    tracked by separate beans/rows in this table. **Exception: the Radxa
    ROCK 4SE's three header I2C buses (i2c2/i2c6/i2c7) are
    hardware-verified** — device ACKs confirmed on all three via a Qwiic
    Button, from a stock (non-`--artifacts-dir`) `gosd build` using the
    published v0.5.0 artifacts (bean `gosd-sz6p`, 2026-07-23), which also
    confirms that DTB-artifact wrinkle resolved for this board. Every other
    board's I2C row remains code-complete-only.

[^gpio]: Every board's kernel already enables the character-device GPIO
    API (`CONFIG_GPIO_CDEV`), so `/dev/gpiochipN` appears at boot with no
    per-board enablement work needed (unlike I2C/SPI, which needed
    device-tree/`config.txt` changes) — bean `gosd-nyad`. `examples/gpioinfo`
    is the worked, cross-board example: a safe-by-default `gpioinfo`(1)-style
    enumeration of every chip/line, with an opt-in (env-var-gated) single-line
    output toggle for confirming wiring. `docs/runtime.md`'s "GPIO, I2C, SPI"
    section documents per-board `gpiochip` numbering and a header-pin →
    (chip, line) example for each board. Same caveat as the rest of this
    table: code-complete and fake-artifact/QEMU-tested, not yet verified
    against a real GPIO device on hardware (that bench step, an LED blink on
    each board, is the one item this bean leaves unchecked). **Exception:
    the Radxa ROCK 4SE's five `gpiochip0`-`gpiochip4` character devices are
    hardware-confirmed to enumerate** via `examples/gpioinfo` (bean
    `gosd-sz6p`, 2026-07-23); the LED-blink line-toggle bench step remains
    unchecked on this board too, same as every other board in this table.

[^spi]: SPI is enabled by default on every board as of bean `gosd-fnza` — no
    build flag needed, and there's no opt-out today. Mechanism differs by
    board family, the same shape as I2C (`gosd-85pt`): the Pi boards gained
    `dtparam=spi=on` in `config.txt` (both chip selects, `/dev/spidev0.0` and
    `/dev/spidev0.1`); the Rockchip boards gained a kernel-build-time
    device-tree patch (`build/boards/radxa-zero-3e/kernel/patches/`,
    `build/boards/nanopi-zero2/kernel/patches/`,
    `build/boards/rock-4se/kernel/patches/`) enabling the header-routed
    `spiN` controller node plus a `spidev` child node per header-routed chip
    select (compatible `rohm,dh2228fv` — a bare `"spidev"` compatible is
    refused by the kernel's spidev driver, see `docs/runtime.md`'s SPI
    section) — same pinned-U-Boot-lacks-`CONFIG_OF_LIBFDT_OVERLAY` reasoning
    as I2C, so this ✅ carries the same "code-complete, fake-artifact-tested,
    not hardware-verified" caveat as the rest of this table, plus the same
    wrinkle: **the Rockchip boards' DTB artifact needs a new artifacts
    release (`v0.4.0`) before a real, non-`--artifacts-dir` build picks up
    the change** — that release, and the follow-up `internal/artifacts.
    Version` bump once it's tagged, are tracked as separate follow-up work,
    not done in this bean. The Radxa Zero 3E only exposes one chip select
    (`/dev/spidev3.0`) — its 40-pin header's physical pin 26, where a Pi's
    CE1 would be, is not connected. Per-board bus and pin numbers are
    documented in `docs/runtime.md`'s "GPIO, I2C, SPI" section;
    `examples/spiloopback` is the worked, cross-board example (a
    jumper-MOSI-to-MISO self-test, since no fixed peripheral is assumed).

[^nanopi-fpc]: The NanoPi Zero2 exposes GPIO on a 30-pin FPC (flex) connector,
    **not** a Raspberry Pi–style 40-pin header — an example written for the
    Pi/Radxa's header pinout will not carry over to this board without
    adjustment.

[^ota]: Over-the-network app updates (app-slot A/B scheme) are designed
    (`docs/design/ab-updates.md`) but not implemented — epic `gosd-vxal`,
    deliberately deferred priority, explicitly gated on v0.2 shipping first.
    No board-specific work is expected here: the design is single,
    board-agnostic mechanism.

---

*An internal-only `qemu-virt` board profile also exists, for CI and local
contributor testing under `qemu-system-aarch64 -M virt` — it is excluded
from `gosd build`'s default board set and from this table because it is not
a real, end-user-flashable board (see `CLAUDE.md`'s locked decisions and
`docs/runtime.md`'s "Testing your app under qemu" section).*
