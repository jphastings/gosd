# Board / feature compatibility

What works on which board, as of the current `main`. This is a snapshot of
repo state (code, kernel configs, and tracked work in beans), not a roadmap —
see `beans list` for what's in flight.

> **Four of the six boards have now been through hardware bring-up**: the
> Radxa ROCK 4SE (bean `gosd-sz6p`, 2026-07-23), the NanoPi Zero2 (bean
> `gosd-odp7`, 2026-07-24), the Raspberry Pi Zero 2W (bean `gosd-m9dj`,
> 2026-07-25, with the WiFi root-cause saga in bean `gosd-anyp`), and the
> Raspberry Pi Zero W (bean `gosd-qltr`, 2026-07-26, whose bring-up shook
> out three kernel bugs: beans `gosd-md4w`, `gosd-1ey5`, `gosd-6nl2`). The
> Radxa Zero 3E's bring-up is in progress (bean `gosd-nlzf`): it boots to
> the app on real hardware with DHCP and mDNS proven, but its remaining
> checklist items (boot-time baseline, power-cycle survival, gadget,
> peripherals) are still open, and its 1.5M-baud serial console needs an
> adapter workaround (see the `gosd build` row's footnote). The Raspberry
> Pi 3B — one image covering the 3B and the 3B+, activated 2026-07-26 with
> its first published artifacts (`artifacts/v0.8.0`, epic `gosd-xhc3`) —
> had its maiden hardware boot the same day, reaching HTTP over wired
> Ethernet first try (on a 3B+; see [^pi3b-eth]), but its formal bring-up
> checklist (bean `gosd-f5xm`) is still open.
>
> Each completed bring-up hardware-proved a common core on that board:
> `gosd build` → flash → boot chain → serial console → network up (Ethernet
> or WiFi) → mDNS + HTTP reachability → repeated power-cycle survival. A ✅
> in a row outside that core still means *code-complete: implemented,
> unit-tested, and (where applicable) QEMU-tested via the internal
> `qemu-virt` profile* — the footnotes are the source of truth for per-row
> hardware verification, and outside that core a cell without a
> bring-up-dated footnote has not been exercised on a real device.
>
> One distinction to keep in mind throughout: several fixes found during
> the Pi bring-ups live in kernel fragments or DTS patches (beans
> `gosd-md4w`, `gosd-1ey5`, `gosd-6nl2`, and `gosd-spjt`'s legacy-gadget
> eviction). Each was hardware-proven via locally built kernels
> (`gosd build-kernel` + `--artifacts-dir`) first; all of them shipped in
> `artifacts/v0.7.0` (pinned since bean `gosd-xo9u`), so ordinary
> `gosd build` images now carry them — what remains open on any of those
> beans is bench re-verification from the published artifacts, noted in
> footnotes where it applies.

| Feature | Raspberry Pi Zero 2W | Raspberry Pi Zero W | Raspberry Pi 3B | Radxa Zero 3E | NanoPi Zero2 | Radxa ROCK 4SE |
|---|---|---|---|---|---|---|
| Image build via `gosd build` | ✅ [^pi-dtb] | ✅ [^armv6-perf] | ✅ [^pi3b-family] | ✅ [^radxa-serial] | ✅ | ✅ |
| Published artifacts (kernel/bootloader) | ✅ | ✅ | ✅ [^pi3b-artifacts] | ✅ | ✅ [^nanopi-artifacts] | ✅ [^rock4se-artifacts] |
| Custom kernel (`gosd build-kernel`) | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] | ✅ [^custom-kernel] |
| Bundle prebuilt static binary (`--with-external`) | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] | ✅ [^with-external] |
| Ethernet | ➖ [^pi-no-eth] | ➖ [^pi-no-eth] | ✅ [^pi3b-eth] | ✅ [^eth-verified] | ✅ [^eth-verified] | ✅ [^eth-verified] |
| WiFi (WPA2-PSK / open) | ✅ [^pi-zero-2w-wifi] | ✅ [^pi-zero-w-wifi] | ✅ [^pi3b-wifi] | ➖ [^no-radio] | ➖ [^nanopi-wifi] | ❌ [^rock4se-wifi] |
| Hidden-SSID WiFi | ✅ [^hidden-ssid] | ✅ [^hidden-ssid] | ✅ [^hidden-ssid][^pi3b-wifi] | ➖ [^no-radio] | ➖ [^nanopi-wifi] | ❌ [^rock4se-wifi] |
| Imager catalog provisioning | ✅ [^pi-tag] | ✅ [^pi-zero-w-tag] | ✅ [^pi3b-tag] | ✅ [^no-filtering] | ✅ [^no-filtering] | ✅ [^no-filtering] |
| `gosd.toml` config (fallback) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| App env vars (`gosd.toml [env]`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| mDNS (`<hostname>.local`) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| SNTP time sync | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Persistent `/data` partition | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] | ✅ [^data-opt-in] |
| Onboard eMMC format/mount (`emmc` package) | ➖ [^no-emmc] | ➖ [^no-emmc] | ➖ [^no-emmc] | ✅ [^emmc] | ✅ [^emmc] | ✅ [^emmc][^rock4se-emmc] |
| Attached disk format/mount (`disk` package) | ✅ [^disk] | ✅ [^disk] | ✅ [^disk] | ✅ [^disk] | ✅ [^disk] | ✅ [^disk] |
| exFAT on attached disks | ✅ [^exfat] | ✅ [^exfat] | ✅ [^exfat] | 🚧 [^exfat] | 🚧 [^exfat] | ✅ [^exfat] |
| USB gadget (serial/Ethernet/mass storage) | ✅ [^usb-gadget][^pi-dwc2] | ✅ [^usb-gadget][^pi-dwc2] | ➖ [^pi3b-no-gadget] | ✅ [^usb-gadget] | ❌ [^nanopi-usb] | ✅ [^usb-gadget][^rock4se-otg] |
| NVMe SSD (M.2) + exFAT | ➖ [^no-m2] | ➖ [^no-m2] | ➖ [^no-m2] | ➖ [^no-m2] | ➖ [^no-m2] | ✅ [^rock4se-nvme] |
| I2C | ✅ [^i2c] | ✅ [^i2c] | ✅ [^i2c] | ✅ [^i2c] | ✅ [^i2c][^nanopi-fpc] | ✅ [^i2c] |
| GPIO | ✅ [^gpio] | ✅ [^gpio] | ✅ [^gpio] | ✅ [^gpio] | ✅ [^gpio][^nanopi-fpc] | ✅ [^gpio] |
| SPI | ✅ [^spi] | ✅ [^spi] | ✅ [^spi] | ✅ [^spi] | ✅ [^spi][^nanopi-fpc] | ✅ [^spi] |
| Audio out (custom kernel) | ✅ [^audio] | ✅ [^audio] | ✅ [^audio] | 🚧 [^audio] | ➖ [^audio] | ✅ [^audio][^rock4se-audio] |
| OTA app updates | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] |

**Legend:** ✅ implemented · 🚧 planned or in-progress · ➖ not applicable
(no matching hardware) · ❌ not supported (with a reason below).

## Footnotes

[^rock4se-audio]: **Hardware-verified 2026-07-30** (bean `gosd-cfkd`): a
    tone from `examples/chime` was *heard* out of the ROCK 4SE's 3.5 mm jack —
    the only board on this row where sound has been confirmed by ear rather
    than by compiling. The path is the analog recipe (ES8316 codec on i2c1,
    i2s0, no DRM: +1,146,880 bytes / +1.68%). It needed the `sound` package's
    audibility pass, because the codec powers up silent in two separate ways:
    `DAC Playback Volume` and `Headphone Mixer Volume` both sat at 0, and the
    `Left`/`Right Headphone Mixer ... DAC Switch` elements were off, so DAPM
    never powered the output stage. The pass raised the two volumes and set
    the two switches, and left the capture-side `Differential Mux` alone. The
    board's HDMI audio path (the other recipe variant, which costs DRM) is
    still unheard.

[^audio]: Sound is deliberately absent from every published GoSD kernel
    (`# CONFIG_SOUND is not set`), so this row is about a `gosd build-kernel`
    recipe, not the stock image — the same stance as display/DRM, which has no
    row here at all. **`docs/sound.md` is the guide**; the public `sound`
    package plays the frames (talking the kernel's ALSA PCM ioctl interface
    directly, since a GoSD image has no `libasound.so.2` to link or `dlopen`)
    and `examples/chime` is the worked example, shipping every recipe below.
    The cells mean three different things, and the difference is the
    verification tier, not the difficulty:
    **✅ = a recipe that a real `gosd build-kernel` run compiled** —
    pi-zero-w and pi-zero-2w (each resulting `kernel.config` checked: ten
    `CONFIG_SND*=y` symbols, `CONFIG_SND_BCM2835=y`, every denied symbol
    absent), costing +104,248 bytes on the Zero W's kernel image and +401,408
    on the Zero 2 W's (+0.63% / +0.71%) against the published
    `artifacts/v0.8.0` kernels; pi-3b is ✅ on the strength of sharing that
    fragment, patch, defconfig and driver, but was not compiled.
    **🚧 = a recipe written against the pinned kernel's Kconfig and device
    trees but never compiled and never measured** — both Rockchip boards
    (bean `gosd-lrxz`). **No board of any tier has been heard on hardware
    yet.** Per-board hardware and cost differ sharply: Pi audio needs neither
    DRM nor ASoC (`snd_bcm2835` talks to the VideoCore firmware over VCHIQ and
    binds to a VCHIQ bus device, so no device-tree patch either), and on the
    Pi the HDMI ALSA card exists only if the firmware sees a display at probe
    time, so the cable must be connected before power-up. The Pi Zeros have
    HDMI and no jack; the Pi 3B adds a PWM-driven 4-pole jack; the ROCK 4SE
    has HDMI plus a 4-ring jack on an ES8316 codec (I2C1, 0x11) whose ASoC
    path needs no DRM, which is why it gets a cheap analog-only recipe as well
    as a DRM-bearing HDMI one; the Radxa Zero 3E is HDMI-only (no jack, no
    codec) so its only recipe pulls in DRM. Neither Rockchip board needs a DTS
    patch — mainline already enables the codec, I2S and card nodes — so
    neither recipe touches `build/boards/`, and no artifacts release is
    involved. The **NanoPi Zero2 has no HDMI connector at all** and its RK3528
    has no `i2s`/`spdif`/`hdmi` node in the pinned kernel's device tree,
    leaving only header pins with no driver to reach them, hence ➖. Epic
    `gosd-qkbl` records the research and the open question of whether sound
    should move into the stock kernels (bean `gosd-ette`).

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
    `kernel.config` with every expected symbol `=y`. Locally built kernels
    from this pipeline have also now run on real hardware: the Pi Zero W
    bring-up (bean `gosd-qltr`, 2026-07-26) bench-proved each of its three
    kernel fixes (beans `gosd-md4w`, `gosd-1ey5`, `gosd-6nl2`) with a
    `gosd build-kernel` rebuild fed into a `--artifacts-dir` image — the
    same flow a developer's custom kernel takes. The flagship DVB-T example
    itself hasn't been run on a physical board.

[^with-external]: `gosd build --with-external <path>[:<dest>]` (repeatable)
    bundles a prebuilt, fully static executable into the initramfs at mode
    0755 (see `docs/runtime.md#bundling-a-companion-binary---with-external`).
    Build-time validation (`debug/elf`) checks each binary's ELF class/
    machine against the board's architecture and rejects a dynamically
    linked binary (`PT_INTERP` present) with an actionable error, so this
    row is code-complete and fake-artifact-tested against real
    cross-compiled static Go binaries for both arm64 and armv6 boards; it
    hasn't been exercised on physical hardware yet. The
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
    unverified on this board — though the codepath itself is now
    hardware-proven on the NanoPi Zero2 (see [^emmc]).

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
    all asserted by the board's kernel fragment — so an M.2 NVMe SSD is
    usable from an app. **Hardware-verified** during bring-up (bean
    `gosd-sz6p`, 2026-07-23) with the actual target SSD (KIOXIA
    XG7000-512): the previously flagged RK3399 PCIe link-training risk
    didn't manifest — the drive enumerated immediately as `/dev/nvme0n1`,
    sustained 256 MiB @ 840 MB/s sequential read, and exFAT mounted via
    `unix.Mount` with data surviving unmount/remount. (The link-training
    timeout logged with the slot empty was confirmed benign probing noise,
    not a real fault.) That bring-up mounted the drive by hand; the
    supported path is now the `disk` package (see [^disk]), which discovers
    the SSD and formats/mounts it whole-device — FAT32 by default, exFAT on
    request, and mounting an existing exFAT volume of the app's own rather
    than wiping it (see [^exfat]). An app only needs `unix.Mount` directly
    if it wants a filesystem GoSD does not write at all.

[^pi3b-family]: **One `pi-3b` image covers the whole Pi 3B family — 3B and
    3B+** (JP's locked decision, epic `gosd-xhc3` / bean `gosd-oq0z`): the
    boot partition ships both models' DTBs (`bcm2710-rpi-3-b.dtb` and
    `bcm2710-rpi-3-b-plus.dtb`, the GPU firmware picks by board revision)
    and the kernel carries both models' USB-Ethernet drivers (see
    [^pi3b-eth]). The one family asymmetry is WiFi — see [^pi3b-wifi]. The
    BCM2837 is the same arm64 SoC family as the Zero 2W, and the board
    boots the same GPU-ROM flow (no U-Boot).

[^pi3b-artifacts]: The Pi 3B's kernel (`kernel8.img` plus both family DTBs)
    is built and published by CI (`pi-3b-kernel` job,
    `.github/workflows/build-artifacts.yml`, bean `gosd-0nl7`) from the
    `artifacts/v0.8.0` release onward, at the same Pi fleet kernel pin as
    the Zeros. There is no bootloader artifact: the Pi's GPU ROM boots the
    FAT partition directly.

[^pi3b-eth]: Onboard wired Ethernet is this board's headline feature vs the
    Zeros, and is **hardware-verified** (2026-07-26, epic `gosd-xhc3`'s
    maiden boot, recorded in bean `gosd-f5xm`): DHCP, mDNS, and HTTP 200
    end-to-end over the wire, first try. The bench board was a **3B+**,
    whose LAN7515 GbE chip came up via `lan78xx` (`CONFIG_USB_LAN78XX=y`,
    asserted in the kernel fragment by bean `gosd-oq0z`); that boot used
    the 3B DTB as a firmware fallback, before bean `gosd-oq0z` shipped the
    3B+'s own DTB. The plain 3B's LAN9514 100Mbit chip uses `smsc95xx`
    (`CONFIG_USB_NET_SMSC95XX=y`, asserted since bean `gosd-ypg1`) — both
    chips self-enumerate on USB, DTB-agnostic, but a plain 3B hasn't been
    on the bench yet, so the 3B half of the family claim is
    code-asserted, not hardware-verified (bean `gosd-f5xm`).

[^pi3b-wifi]: Code-complete, not hardware-verified on this board: the 3B's
    BCM43438 is the same Cypress 43430 blob set as the Pi Zero W's (bean
    `gosd-06kj`), shipped in the initramfs under this board's
    `brcmfmac43430-sdio.raspberrypi,3-model-b.*` alias names, through the
    same `wifiup` stack that is hardware-proven on both Pi Zeros. Family
    caveat: the **3B+**'s WiFi chip is a BCM43455, whose blob set
    (`43455` + `3-model-b-plus` aliases) is NOT yet in this board's
    manifest — WiFi on a 3B+ does not work yet (follow-up recorded in bean
    `gosd-oq0z`); a 3B+ is wired-Ethernet-first for now.

[^pi3b-tag]: Raspberry Pi Imager's official catalog
    (`downloads.raspberrypi.org/os_list_imagingutility_v4.json`, fetched
    and inspected directly on 2026-07-26 for this board's activation — see
    `internal/catalog.boardImagerDeviceTags`) defines the "Raspberry Pi 3"
    device (description: "Raspberry Pi 3 Model A+ / B / B+ and Compute
    Module 3 / 3+") with tags `["pi3-64bit", "pi3-32bit"]`. GoSD's pi-3b
    image is arm64, so its catalog entry carries `pi3-64bit` only. The
    same shared-namespace consequence as [^pi-tag], in both directions:
    the "Raspberry Pi Zero 2 W" device carries exactly the same tags, so
    a GoSD pi-3b entry also appears when a user selects **Raspberry Pi
    Zero 2 W** in Imager's device-filter step — just as the pi-zero-2w
    entry already appears under "Raspberry Pi 3".

[^pi3b-no-gadget]: USB gadget mode is structurally impossible on the Pi 3B
    family — a hardware limitation, not a GoSD gap (epic `gosd-xhc3`
    locked decision): the BCM2837's only USB port is hard-wired through
    the onboard hub/Ethernet chip (LAN9514 on the 3B, LAN7515 on the 3B+),
    so the controller can never be put into peripheral mode and no UDC can
    ever exist for the `gadget` package to bind. `gosd build
    --board=pi-3b --usb-gadget` fails fast with an actionable error (bean
    `gosd-5pnr`'s capability check), and the board's `config.txt` template
    has no dwc2-overlay branch at all.

[^pi-no-eth]: Neither the Raspberry Pi Zero 2 W nor the original Zero W has
    an onboard Ethernet port (WiFi only) — this is a hardware limitation of
    both boards, not a GoSD gap. `gosd-init`'s wired-networking code
    (`cmd/gosd-init/internal/netup`) matches any `eth*`/`end*`/`enp*`
    interface generically, so a USB-Ethernet adapter on the micro-USB OTG
    port would likely work through the same DHCP path, but this is untested
    and not a documented/supported configuration.

[^eth-verified]: Wired Ethernet is hardware-verified on all three Rockchip
    Ethernet boards during their bring-ups (the Pi 3B's is covered
    separately, see [^pi3b-eth]): DHCP lease, mDNS resolution from macOS,
    and HTTP reachability (over IPv6, incidentally proving both stacks) on
    the ROCK 4SE (bean `gosd-sz6p`, 2026-07-23) and the NanoPi Zero2 (bean
    `gosd-odp7`, 2026-07-24, which also observed the SNTP clock sync);
    DHCP lease and mDNS resolution on the Radxa Zero 3E (bean `gosd-nlzf`,
    2026-07-24 — an HTTP check with a serving app is among that bring-up's
    open items).

[^armv6-perf]: The Zero W's BCM2835 has a single ARM1176JZF-S core at armv6
    (`GOARCH=arm GOARM=6`, no NEON) — a fraction of the Zero 2 W's quad-core
    64-bit Cortex-A53. Both the app and gosd-init are cross-compiled for this
    target (see the "Target" locked decision in `CLAUDE.md` and bean
    `gosd-2j6z`'s per-arch build pipeline), so this is a real, expected
    performance ceiling for any CPU-bound app logic on this board, not a
    missing optimization — plan accordingly for anything heavier than
    GPIO/network I/O. Borne out on the bench: the Zero W reaches
    HTTP-over-WiFi noticeably later than the Zero 2W on the same network
    (bean `gosd-qltr`'s timings vs bean `gosd-m9dj`'s).

[^pi-dtb]: Until bean `gosd-f59k` (fixed 2026-07-25), the Pi Zero 2W's boot
    partition omitted its device tree blob (`bcm2710-rpi-zero-2-w.dtb`) —
    firmware and `kernel8.img` loaded, then the kernel hung before any
    console output (no device tree = no drivers, no UART), a silent failure
    with a healthy-looking ACT LED. Found during the Zero 2W's first
    hardware bring-up (bean `gosd-m9dj`, 2026-07-24); the Pi Zero W board
    profile already copied its DTB correctly and was unaffected. Fixed by
    adding the DTB to `pizero2w`'s `Artifacts()`/`BootFiles()`, with an
    integration-test assertion added so a regression fails CI —
    hardware-confirmed 2026-07-25, when the first flash from the fixed
    build booted with no hand-patch (bean `gosd-m9dj`, session 2).

[^radxa-serial]: The Zero 3E's 1500000-baud debug console is unreliable on
    CP210x/PL2303-family USB-serial adapters — bytes garble (slow rising
    edges read back with high bits skewed set) at that rate, while the same
    adapter/wires read the ROCK 4SE and NanoPi Zero2 cleanly at the same
    1500000 (bean `gosd-nlzf`, 2026-07-24 bench debugging; reproduced on two
    Zero 3E units and both GoSD and Armbian, so it's an adapter/UART
    interaction, not board damage). Radxa's own serial documentation notes
    the same limitation and recommends a CH340-based adapter/cable instead.
    Two workarounds need no reflash: `gosd build --console-baud 115200` at
    build time, or hand-editing `console=ttyS2,...` in
    `extlinux/extlinux.conf` on the flashed `GOSD-BOOT` partition — either
    way, U-Boot's own output stays at its compiled-in 1500000 regardless,
    since that's baked into the pinned U-Boot binary, not something `gosd
    build` renders. See `docs/runtime.md`'s "Serial console baud rate"
    section for the full writeup; a kernel-side drive-strength fix (bean
    `gosd-zp9s`) remains open, pending bench verification.

[^pi-zero-2w-wifi]: Hardware-verified 2026-07-25 (bean `gosd-m9dj`):
    WPA2-PSK join via the firmware-offloaded handshake, DHCP, mDNS and HTTP
    all work on a real Zero 2W. Getting there took a two-bench-day
    root-cause hunt (bean `gosd-anyp`): wifiup's nl80211 CONNECT omitted
    the `netlink.Request` flag, which the kernel silently acks-and-skips —
    so every join ever issued was a no-op that logging disguised as an
    associate/deauth loop. The fix is gosd-init code (plus a CI regression
    test pinning the netlink flags and attribute sequence), so it needed no
    artifact release. One cosmetic kernel-fragment fix shipped in
    `artifacts/v0.7.0`: bean `gosd-6nl2` disables the phantom
    `mac80211_hwsim` radios that made the real radio enumerate as `wlan2`.

[^pi-zero-w-wifi]: The Zero W's WiFi/BT combo chip is a single revision,
    plain BCM43430 (unlike the Zero 2 W's three chip revisions) — the
    board's kernel enables `CFG80211`/`BRCMFMAC`, and its board profile
    (`internal/boards/pizerow`) ships the matching firmware blob (fetched
    from upstream's Cypress-branded `cyfmac43430-sdio.*`, per bean
    `gosd-06kj`'s findings) plus its board-specific alias names, flattened
    into `/lib/firmware/brcm` the same way pi-zero-2w's are.
    Hardware-verified 2026-07-26 (bean `gosd-qltr`): WPA2-PSK join via the
    firmware-offloaded handshake, DHCP, mDNS and HTTP all work on a real
    Zero W — after three kernel fixes found during bring-up (`gosd-md4w`
    console, `gosd-1ey5` SD DMA, `gosd-6nl2` phantom-radio/SDIO-controller),
    all shipped in `artifacts/v0.7.0`.

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
    section. Hardware-exercised on both Pi Zeros during the usbwebsite
    bench session (beans `gosd-4ajn`/`gosd-spjt`, 2026-07-26): on each
    board the partition was found, mounted read-write, and its contents
    served over HTTP.

[^no-emmc]: No Raspberry Pi board in this table has onboard eMMC — a
    hardware limitation of these boards, not a GoSD gap. The `emmc`
    package's `FormatAndMount` returns `ErrNoEMMC` on these boards. An app that wants
    USB-shareable storage here uses the SD card's `GOSD-DATA` partition
    instead (`gosd build --data-size`): `examples/usbwebsite` falls back to
    it on `ErrNoEMMC` automatically, serving from gosd-init's `/data` mount
    and handing the raw partition to `gadget.MassStorage` when a computer is
    attached (bean `gosd-4ajn`). Hardware-exercised on both Pi Zeros
    (2026-07-26): the `ErrNoEMMC` fallback found `GOSD-DATA` and served it
    over HTTP on each board; the mass-storage half waits on the gadget fix
    (see [^pi-dwc2]).

[^emmc]: The `emmc` package (public API, see `docs/runtime.md`'s "Onboard
    eMMC" section) auto-discovers the board's onboard eMMC — distinguishing
    it from the booted microSD card, which is never a format target — and
    formats it with a whole-device FAT filesystem the first time it's seen
    blank, mounting-only on every run after that. It carries the same
    FAT-only caveats as the `/data` partition (no unix permissions/symlinks,
    not power-loss-robust; write with the temp-file+fsync+rename pattern).
    **Hardware-verified on the NanoPi Zero2** (bean `gosd-odp7`,
    2026-07-24): first hardware validation of the detect/format/mount path,
    and of serving eMMC-hosted content over HTTP via `examples/usbwebsite`
    adopting the labelled volume non-destructively. That bring-up also
    filed (and fixes landed for) bean `gosd-pcwl` — gosd-init's
    boot-partition probe could be shadowed by an eMMC carrying a valid FAT
    first partition — and bean `gosd-4jn5`, usbwebsite crash-looping on an
    eMMC with prior content. `examples/emmcstorage` is the worked example.
    On the ROCK 4SE the *no-module-fitted* branch (`ErrNoEMMC`) is
    hardware-confirmed (bean `gosd-sz6p`, 2026-07-23, see [^rock4se-emmc]),
    and the Radxa Zero 3E units on the bench have no onboard eMMC either,
    so its bring-up exercised the same `ErrNoEMMC` branch (bean
    `gosd-nlzf`) — the format/mount path remains code-complete-only
    everywhere except the NanoPi Zero2.

[^disk]: The `disk` package (public API, see `docs/runtime.md`'s "Attached
    disk storage" section, bean `gosd-yggd`) is the general-purpose sibling
    of `emmc`: it discovers whatever mass storage is attached and is *not*
    the media the board booted from — an NVMe SSD, a USB drive, an SD card
    in a reader — and formats/mounts it under the same whole-device-FAT,
    label-keyed, idempotent rules. Every board in this table has a USB host
    port, so the USB-drive case applies to all of them; NVMe applies only to
    the ROCK 4SE (see [^rock4se-nvme]). Carries the same caveat as most rows in this
    table: ✅ means code-complete and unit-tested, with hardware
    verification tracked separately — bean `gosd-yggd`'s bench checklist
    (rock-4se NVMe discover/format/mount/gadget-share, plus a USB drive on
    any board) is what confirms it on real hardware.
    FAT32 is what it formats by default; a disk arriving pre-formatted as
    exFAT (how most SSDs and USB drives ship) is now mounted rather than
    refused when its label matches the app's — see [^exfat].

[^exfat]: `disk` reads, mounts and writes exFAT, not only FAT32 (bean
    `gosd-1ici`), which is what lifts FAT32's hard 4 GiB per-file ceiling on
    a large SSD. Two things use it: a drive that arrived exFAT-formatted and
    carries the app's own label is mounted as it is instead of being wiped,
    and `disk.FormatAndMountWith(…, disk.Options{Filesystem: disk.ExFAT})`
    formats one deliberately. The formatter is pure Go, written against the
    Microsoft exFAT specification (`internal/diskfmt`), since `go-diskfs` has
    no exFAT support.
    What varies by board is only the *kernel*, which must have
    `CONFIG_EXFAT_FS`. Both Pi Zeros, the Pi 3B and the ROCK 4SE have it in
    their **published** artifacts today, so exFAT works on those boards
    now — for the Pi boards it was inherited from their defconfig rather
    than asserted, which this bean fixes by pinning it in their fragments
    (no change to the compiled kernel, only to what a future trim may cut).
    The Radxa Zero 3E and NanoPi Zero2 published kernels have
    `# CONFIG_EXFAT_FS is not set`; their fragments now enable it, so it
    reaches released artifacts at the next artifacts version — until then
    asking for exFAT on those two boards fails with `disk.ErrUnsupportedFS`
    before the disk is touched, which is deliberate (see [^disk] for the
    package, and `docs/runtime.md` for the API). Hardware verification of
    the formatter is tracked in bean `gosd-1ici`'s bench checklist.

[^usb-gadget]: The kernel config for USB gadget mode (DWC2 on both Pi
    Zeros, DWC3 on the Radxa boards; `CONFIG_USB_GADGET`, configfs,
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
    gadget-capable board's recorded published `kernel.config` already
    carries it, but only
    incidentally — inherited from the defconfig baseline, asserted by no
    kernel fragment or `internal/kernelspec` `RequiredY` list — so the
    *guaranteed* enablement lands when the fragments gain it explicitly at
    the next fleet kernel tag bump (never a single-board bump). The
    exception is the Radxa ROCK 4SE (epic `gosd-cuym`), which asserts it in
    its stock kernel fragment and `RequiredY` from the start. Hardware
    status per board: **the ROCK 4SE's CDC-ACM path is hardware-verified**
    (bean `gosd-sz6p`, 2026-07-23, see [^rock4se-otg]); the Pi Zeros
    reached the bench on 2026-07-26 and hit two gadget blockers, one fixed
    in `gosd build` and one pending an artifacts release (see [^pi-dwc2]);
    the Radxa Zero 3E's gadget test is an open item on its bring-up (bean
    `gosd-nlzf`). USB mass storage end-to-end remains unverified on real
    hardware on every board.

[^pi-dwc2]: Until bean `gosd-spjt` (image-side fix landed 2026-07-26),
    neither Pi Zero could actually reach gadget mode on real hardware
    despite this row's ✅: `--usb-gadget` rendered
    `dtoverlay=dwc2,dr_mode=peripheral` into config.txt, but nothing shipped
    `overlays/dwc2.dtbo` to the boot partition, so `start.elf` skipped the
    directive silently and no UDC ever appeared (found during the Zero W/2W
    bench bring-up under bean `gosd-4ajn`). Fixed by pinning the overlay
    (raspberrypi/firmware, same commit as the GPU boot firmware) in both
    boards' manifests and shipping it on `--usb-gadget` builds — effective
    in any `gosd build` from that commit, with no artifact release needed.
    A second blocker rides the next artifacts release: both Zeros'
    *published* kernels carry the kernel's legacy test gadgets built-in
    (defconfig `=m` promoted by the no-modules build), and the first of
    them — "Gadget Zero", `0x0525`/`0xa4a0` — claims the board's only UDC
    before the app can apply its own configfs gadget. Both facts are
    hardware-proven (bean `gosd-spjt`, 2026-07-26): with the overlay in
    place the bench Zero W got a UDC and enumerated on the host — as
    Gadget Zero. The kernel fragments now evict the whole
    `drivers/usb/gadget/legacy` family, shipped in `artifacts/v0.7.0`;
    until the bench re-run tracked in `gosd-spjt`, an app's own gadget
    (mass storage included) remains unverified on both Pi Zeros.

[^nanopi-usb]: The RK3528 SoC has no USB controller DT node in any numbered
    mainline kernel release as of the pinned tag (v6.18.37) — the `dwc3` node
    and the board's USB-enable commit exist only on Linux's development
    `master`, not yet in a release. Confirmed directly against the pinned
    kernel source (bean `gosd-rqx8`), and confirmed on the bench during
    bring-up: no UDC at the pinned kernel, with the app degrading
    gracefully to serve mode (bean `gosd-odp7`). Consequence: the NanoPi
    Zero2 has no USB at all — host or gadget — until a future fleet-wide
    kernel version bump picks up that commit; Ethernet, SD/eMMC, and
    serial console are unaffected. Recheck when bumping the pinned kernel tag. `gosd build
    --usb-gadget` refuses to build for this board (bean `gosd-5pnr`) rather
    than producing an image whose app can never find a UDC. Update
    (bean `gosd-36yy`, 2026-07-24): identified precisely — SoC node in
    commit `5f3ae9b12a6c` ("arm64: dts: rockchip: Add USB nodes for
    RK3528"), board enablement in commit `ff660109f412` ("arm64: dts:
    rockchip: Enable USB 2.0 ports on NanoPi Zero2"), both 2026-06-02. Both
    are present in mainline's `v7.2-rc4` (an unreleased pre-release for the
    next major kernel version) but in no tagged, numbered release yet — not
    even the already-superseded v7.0.x/v7.1.x lines picked them up, as
    expected for a new-hardware DT addition (not a stable-eligible bug fix).
    The fleet kernel tag bump stays deferred until a real release ships
    them; gosd-36yy has the full evidence trail and a pre-baked plan
    (including the DTS `dr_mode = "peripheral"` patch this board will need)
    for when it does.

[^i2c]: I2C is enabled by default on every board as of bean `gosd-85pt` — no
    build flag needed, and there's no opt-out today. Mechanism differs by
    board family: the Pi boards gained `dtparam=i2c_arm=on` in `config.txt`;
    the Rockchip boards gained a kernel-build-time device-tree patch
    (`build/boards/radxa-zero-3e/kernel/patches/`,
    `build/boards/nanopi-zero2/kernel/patches/`,
    `build/boards/rock-4se/kernel/patches/`) enabling the header-routed
    `i2cN` controller node, since the pinned U-Boot on all three doesn't
    support `CONFIG_OF_LIBFDT_OVERLAY`/extlinux `fdtoverlays` (checked
    directly against all three defconfigs) — so this ✅ carries a
    "code-complete, fake-artifact-tested, not hardware-verified" caveat.
    (An earlier wrinkle — the Rockchip DTB artifacts needing a new
    artifacts release to carry these patches — is resolved: the published
    releases have shipped the patched DTBs since before the current pin,
    with hardware evidence in the ROCK 4SE and NanoPi Zero2 bring-up
    logs.) Per-board bus and pin
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
    (chip, line) example for each board. Caveat: code-complete and
    fake-artifact/QEMU-tested, not yet verified
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
    not hardware-verified" caveat as I2C, and the same release wrinkle,
    resolved the same way: the published artifacts have shipped the patched
    DTBs since before the current pin (the NanoPi Zero2 bring-up's boot
    log, from published artifacts, shows its patched `spidev` nodes
    probing). The Radxa Zero 3E only exposes one chip select
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
