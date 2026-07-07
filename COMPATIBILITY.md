# Board / feature compatibility

What works on which board, as of the current `main`. This is a snapshot of
repo state (code, kernel configs, and tracked work in beans), not a roadmap —
see `beans list` for what's in flight.

> **No board has been through hardware bring-up yet.** Every ✅ below means
> *code-complete: implemented, unit-tested, and (where applicable)
> QEMU-tested via the internal `qemu-virt` profile* — not "verified on a real
> device." Hardware bring-up for the Pi Zero 2W and Radxa Zero 3E is tracked
> by beans `gosd-m9dj` and `gosd-nlzf`; until those close, treat every ✅ as
> "should work" rather than "confirmed working." The Pi Zero W is the same:
> code-complete and fake-artifact-tested (bean `gosd-et0q`), no bring-up bean
> filed yet.

| Feature | Raspberry Pi Zero 2W | Raspberry Pi Zero W | Radxa Zero 3E | NanoPi Zero2 (planned) |
|---|---|---|---|---|
| Image build via `gosd build` | ✅ | ✅ [^armv6-perf] | ✅ | 🚧 [^board-profile] |
| Published artifacts (kernel/bootloader) | ✅ | ✅ | ✅ | 🚧 [^nanopi-artifacts] |
| Ethernet | ➖ [^pi-no-eth] | ➖ [^pi-no-eth] | ✅ | 🚧 [^board-profile] |
| WiFi (WPA2-PSK / open) | ✅ | ✅ [^pi-zero-w-wifi] | ➖ [^no-radio] | ➖ [^nanopi-wifi] |
| Hidden-SSID WiFi | ✅ [^hidden-ssid] | ✅ [^hidden-ssid] | ➖ [^no-radio] | ➖ [^nanopi-wifi] |
| Imager catalog provisioning | ✅ [^pi-tag] | ✅ [^pi-zero-w-tag] | ✅ [^no-filtering] | 🚧 [^board-profile] |
| `gosd.toml` config (fallback) | ✅ | ✅ | ✅ | 🚧 [^board-profile] |
| mDNS (`<hostname>.local`) | ✅ | ✅ | ✅ | 🚧 [^board-profile] |
| SNTP time sync | ✅ | ✅ | ✅ | 🚧 [^board-profile] |
| Persistent `/data` partition | ✅ | ✅ | ✅ | 🚧 [^board-profile] |
| USB gadget (serial/Ethernet) | ✅ [^usb-gadget] | ✅ [^usb-gadget] | ✅ [^usb-gadget] | ❌ [^nanopi-usb] |
| GPIO / I2C / SPI | 🚧 [^gpio] | 🚧 [^gpio] | 🚧 [^gpio] | 🚧 [^gpio][^nanopi-fpc] |
| OTA app updates | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] | 🚧 [^ota] |

**Legend:** ✅ implemented · 🚧 planned or in-progress · ➖ not applicable
(no matching hardware) · ❌ not supported (with a reason below).

## Footnotes

[^board-profile]: The NanoPi Zero2's board profile (registering it with
    `internal/boards`, wiring `extlinux.conf`, the bootloader raw-writes, and
    the artifact pipeline entry) hasn't been written yet — bean `gosd-wskc`,
    blocked on the U-Boot pipeline (`gosd-f39b`) and the kernel build
    (`gosd-rqx8`, in progress). Until it lands, `gosd build --board=nanopi-zero2`
    doesn't exist as an option at all. Every row marked 🚧 for this reason is
    otherwise board-agnostic gosd-init functionality (mDNS, SNTP, `gosd.toml`,
    the `/data` partition, and Imager catalog output all run the same way
    regardless of board) that will apply automatically once the board profile
    exists — it isn't separate work per feature.

[^nanopi-artifacts]: The NanoPi Zero2's kernel and U-Boot are both built and
    published by CI (`nanopi-zero2-kernel` and `nanopi-zero2-uboot` jobs,
    `.github/workflows/build-artifacts.yml`). U-Boot is pinned to
    **`v2026.07-rc5`** rather than a final release: mainline U-Boot's
    dedicated `nanopi-zero2-rk3528_defconfig` is new in the v2026.07 cycle
    and wasn't in any prior tagged release, and JP asked to pin the latest
    rc now rather than wait for the final tag so this board is
    hardware-testable sooner (bean `gosd-f39b`'s amended gate). Re-pinning to
    the final `v2026.07` release once it ships is an open item on that bean.

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

[^usb-gadget]: The kernel config for USB gadget mode (DWC2 on both Pi
    boards, DWC3 on the Radxa; `CONFIG_USB_GADGET`, configfs, ACM/ECM/RNDIS
    functions) is already enabled on all three kernels. The pure-Go configfs
    gadget library (package `gadget`, a public v0.3 API surface) is
    implemented and unit-tested against a fake filesystem, with CDC-ACM
    serial gadget mode working end to end (`gosd build --usb-gadget`, see
    `examples/usbserial` and `docs/runtime.md`'s "USB gadget mode" section)
    — bean `gosd-uo9f`. USB Ethernet (ECM/RNDIS) isn't built yet (bean
    `gosd-30jz`). Like every other ✅ in this table, this means code-complete
    and unit-tested, not hardware-verified: no on-device USB enumeration has
    been tried on any board yet, blocked on hardware bring-up (`gosd-m9dj`,
    `gosd-nlzf`), which are themselves blocked on acquiring a bring-up kit
    (`gosd-s4t4`).

[^nanopi-usb]: The RK3528 SoC has no USB controller DT node in any numbered
    mainline kernel release as of the pinned tag (v6.18.37) — the `dwc3` node
    and the board's USB-enable commit exist only on Linux's development
    `master`, not yet in a release. Confirmed directly against the pinned
    kernel source (bean `gosd-rqx8`). Consequence: the NanoPi Zero2 has no
    USB at all — host or gadget — until a future fleet-wide kernel version
    bump picks up that commit; Ethernet, SD/eMMC, and serial console are
    unaffected. Recheck when bumping the pinned kernel tag.

[^gpio]: All three boards' kernels already enable the character-device GPIO
    API (`CONFIG_GPIO_CDEV`), I2C, and SPI drivers, so `/dev/gpiochipN`,
    `/dev/i2c-*`, and `/dev/spidev*` are expected to appear at boot. No
    GoSD-side wiring exists yet beyond documenting the libraries to use
    (`docs/runtime.md`) — a worked, board-tested example and per-board pin
    documentation is tracked by bean `gosd-rsrd`, blocked on hardware
    bring-up.

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
