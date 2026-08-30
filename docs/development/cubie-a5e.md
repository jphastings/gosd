# Developing for the Radxa Cubie A5E (`cubie-a5e`)

Bench/bring-up knowledge from board-support work (epic `gosd-h1wv`) that
isn't captured elsewhere. Locked design decisions — the sunxi boot chain,
the TF-A fork pin, the MUSB gadget controller, and the headline DRAM-variant
call — live in CLAUDE.md; this file is for things a future agent or
developer would otherwise have to rediscover by hand, in particular the
three real hardware bugs this board's bring-up found and fixed.

## The 1GB DRAM failure (bean `gosd-84b8`): how to read the failure and why it can't be fixed at our pin

The board halts in SPL with:

```
U-Boot SPL 2026.04 (Aug 07 2026 - 23:50:00 +0000)
DRAM:DRAM test failure at address 0x6fffffc0
 0 MiB
### ERROR ### Please RESET the board ###
```

The failure address decodes exactly, and decoding it is what turns "DRAM is
broken" into "the vendor calibration table is wrong for this chip":
`mctl_calc_size()` (`arch/arm/mach-sunxi/dram_dw_helpers.c`) computes
`0x6fffffc0 = 0x40000000 + (1024MiB × 3/4) − 64`. That's the auto-sizing
probe reading back a test pattern at the 768MiB boundary (and its −64B
fallback) of a correctly-autodetected 1GiB array — so autodetection got the
*size* right and then a data-integrity check failed at the top of the
range. There's no `panic("This DRAM setup is currently not supported")`
anywhere in the log, which is the tell that training itself succeeded; this
is a data-integrity failure under the tuned parameters, not a dead
controller or a wrong offset.

The sequence matters for anyone re-deriving this: `sunxi_dram_init()` first
detects geometry at a conservative `clk=360` using the driver's own safe
LPDDR4 timings, then **re-initialises the controller at
`CONFIG_DRAM_CLK` (1200 by Kconfig default for `MACH_SUN55I_A523`)** using
the defconfig's `TPR11`/`TPR12` values, and the size check that fails runs
after that second, faster init. `radxa-cubie-a5e_defconfig` ships exactly
one fixed set of these values — U-Boot's own Kconfig help text calls them
"value from vendor DRAM settings," i.e. per-chip calibration data — sourced
from whichever reference unit upstreamed the board. The Cubie A5E ships in
1/2/4GB variants with different LPDDR4 chips, and the upstreamed values
don't suit the 1GB chip.

This is corroborated independently: an Armbian forum thread on a 1GB Cubie
A5E unit reports the same stuck-at-uboot symptom on mainline-derived U-Boot
while Radxa's own vendor image boots the same board, and a community fix
(`Guation/radxa-cubie-a5e-armbian-build@202f1bf`) reads `tpr6`/`tpr10`/
`tpr11`/`tpr12` back off the *working* vendor bootloader — precisely the
fields our defconfig hardcodes. `armbian/build#9764` reports another unit
in the same failure family. **Don't waste time hunting for a newer U-Boot
tag to fix this**: `dram_sun55i_a523.c` and `dram_dw_helpers.c` have no
commits after October 2025, both already ancestors of the v2026.04 pin, and
the three post-v2026.04 defconfig commits (SPI, power LEDs, gmac1) touch no
`CONFIG_DRAM_SUNXI_*` value.

The fix that shipped (`build/boards/cubie-a5e/uboot/dram-1gb.config`,
overriding `TPR6`/`TPR10`/`TPR11`/`TPR12` with the community's 1GB-variant
values, merged by the Dockerfile alongside the other fragments) is
hardware-verified two ways worth knowing about if you're re-deriving
confidence in it rather than trusting the bean: the repo-built `.config` is
byte-for-byte identical (2117 lines, clean `diff`) to a config hand-staged
directly on the board earlier in the same session, and a repo-built binary
took the board through `DRAM: 1024 MiB` → BL31 → U-Boot → extlinux →
kernel → gosd-init → `/app`, with `/data` adopted across a reboot.

JP's final call (2026-08-21, recorded in full in `gosd-84b8`) was to keep
the 1GB values and document the 2/4GB variants as unverified rather than
retune towards a hoped-good-everywhere `CONFIG_DRAM_CLK`, on the reasoning
that the only board anyone can test against is the 1GB one — backing off
the clock speed would trade a proven-good config for an unprovable one. The
real fix is a runtime DRAM-variant probe upstream in
`arch/arm/mach-sunxi`, which is out of scope here and not tracked by any
gosd bean — worth knowing if this resurfaces, since it'd otherwise get
rediscovered from scratch.

## U-Boot's preboot USB scan (bean `gosd-uj4l`): find the real Kconfig trigger before touching a `select`

First bench boot took 10.38s SPL-banner-to-`/app`, of which U-Boot alone
was 9.05s — 87% of the boot — and a `starting USB...` → `Bus usb@...: 1 USB
Device(s) found` → `0 Storage Device(s) found` scan inside that took
~4.5s scanning four controllers to find nothing, on a board that only ever
boots from mmc via extlinux.

The instinct is to flip `CONFIG_USB_STORAGE` or `CONFIG_CMD_USB` off in a
fragment — **this does not work and won't build**, because
`arch/arm/Kconfig`'s `ARCH_SUNXI` hard-`select`s both of those (plus
`CONFIG_USB_KEYBOARD`) whenever `DISTRO_DEFAULTS && USB_HOST`, both true
for this board. A `select` always wins over a "not set" merged from a
fragment, so this path is a dead end without also removing real USB host
support or the distro-boot mechanism the mmc/extlinux path itself needs.

The actual trigger is one level removed: `boot/Kconfig`'s `CONFIG_PREBOOT`
defaults to `"usb start"` *because* `CONFIG_USB_KEYBOARD` is selected on
(so a USB keyboard could interrupt autoboot), and `common/main.c`'s
`main_loop()` runs whatever `CONFIG_PREBOOT` says unconditionally, before
the boot-delay countdown even starts. Unlike the selects above, `PREBOOT`
is a plain `default`, which a fragment *can* override — hence
`build/boards/cubie-a5e/uboot/skip-usb-scan.config` setting
`CONFIG_PREBOOT=""`.

Confirming this needed no cross-compiler or Docker: clone U-Boot at the
pinned tag, run `make radxa-cubie-a5e_defconfig`, then
`scripts/kconfig/merge_config.sh -m .config <fragment>` followed by
`make olddefconfig`, entirely with host tools, to see the real resolved
`.config` before/after. That's a fast way to validate any Kconfig-fragment
hypothesis on this board (or any U-Boot board) without waiting on a full
build.

Measured result on hardware (5 clean power cycles both before and after,
same methodology): U-Boot phase 9.05s → **4.50s** (spread 0.03s), a −4.55s
delta that recovers essentially all of the scan's cost and makes this
board the *fastest* in the fleet rather than the slowest. Total SPL→app
went from 10.38s (spread 0.15s) to a 6.98s mean (6.70–7.75s) — the total's
spread widened because the remaining variance sits after the kernel
starts and this re-measurement used `examples/hello` rather than the
original bring-up image, so treat the U-Boot-phase delta as the clean
apples-to-apples number and the total as a range.

This is **not assumed to generalize**: the Rockchip boards (rock-4se,
nanopi-zero2, radxa-zero-3e) don't share the `ARCH_SUNXI` →
`USB_KEYBOARD` → `PREBOOT="usb start"` chain that caused this on sunxi, so
whether they pay the same cost — and what their own resolved
`CONFIG_PREBOOT` even is at their pin — has to be established fresh per
board. That's tracked separately as bean `gosd-ylkv` (no parent, spans
three boards); don't port this board's fix or its number across without
redoing the Kconfig-resolution check.

## USB gadget vs. the two host controllers sharing its phy (bean `gosd-3io0`)

`--usb-gadget` produced a working-*looking* image — `gosd-init` applied the
gadget, `/dev/ttyGS0` existed — that could never actually enumerate on a
host. The signature that gave it away: a purpose-built probe read
`/sys/class/udc/*/state` as `not attached` forever, meaning the peripheral
controller itself saw no host activity on the wire, while the Mac's USB
tree stayed byte-identical across a power cycle (no vendor ID, no new
`/dev/cu.usbmodem*`) — not a failed enumeration attempt, no enumeration
attempt at all.

Root cause, read directly from the kernel log rather than guessed:

```
[0.866933] phy phy-4100400.phy.0: Changing dr_mode to 1     <- 1 = USB_DR_MODE_HOST
[0.866963] ehci-platform 4101000.usb: EHCI Host Controller  <- ehci0 probing, 30us later
```

`sun55i-a523.dtsi`'s `ehci0` (`usb@4101000`) declares `phys = <&usbphy 0>`
— **the same phy index the `usb_otg` MUSB node uses**. sunxi's phy0 is a
single dual-route mux: it goes to MUSB (peripheral) *or* to ehci0/ohci0
(host), never both. The board DTS enables all three
(`usb_otg { dr_mode = "peripheral"; }`, `ehci0`, `ohci0`), so at boot
`ehci0`'s probe calls `phy_set_mode(PHY_MODE_USB_HOST)` a few dozen
microseconds after the phy comes up and simply wins the race — MUSB still
gets its UDC (which is why `Apply()` "succeeds" and `/dev/ttyGS0` exists),
but the physical D+ line is never routed to it.

Normally an OTG port arbitrates this via ID/VBUS pin detection, but the
board genuinely has none: the DTS's own comments say USB0_ID and
USB-VBUSDET both just read `VBUS_5V` (always-on) rather than anything
connector-state-dependent, and the AXP717C PMIC's CC-pin logic isn't wired
to the USB-C connector at all. With no detection hardware, there is no
arbitration — probe order alone decides, and host always wins.

Ruled out before landing on this explanation (don't re-spend time on any
of these): a different data cable, a different physical port (confirmed
working with a known-good USB device on the same port), a wrong-connector
assumption (Radxa's own spec: the Type-C is OTG+power, the Type-A is
host-only — nothing to swap), the kernel config (`CONFIG_USB_MUSB_HDRC`/
`DUAL_ROLE`/`SUNXI` all `=y`, consistent with the UDC existing and
accepting the gadget), and gosd's own `gadget/` package (bound the UDC and
materialized configfs correctly; not implicated).

**The fix shipped is a variant DTB, not a patch to the stock one** — the
same shape as the Pi boards' flag-gated `dwc2.dtbo`. The stock DTS's
`ehci0`/`ohci0` legitimately serve users powering the board off its GPIO
5V pins (where the USB-C becomes a real host port), so disabling them
unconditionally would silently remove that for everyone. Instead the
kernel build emits **two** DTBs — stock and `sun55i-a527-cubie-a5e-gadget.dtb`
(host controllers on that phy disabled) — and `--usb-gadget` selects which
one an image ships. The trade this locks in: **a gadget-mode image loses
USB-C host capability**; the USB 3.0 Type-A port (its own separate
ehci1/ohci1) is unaffected either way.

This landing needed three PRs rather than one, and the reason generalizes
to any future "ship a new DTB variant" board change: `internal/kernelspec`'s
outputs-vs-artifacts test resolves every ref a board lists *eagerly*, so
naming a DTB the currently-released artifacts don't yet carry fails *every*
cubie-a5e build, not just gadget ones. The sequence that avoids that: (1)
land the DTS patch + kernelspec output + an honest interim refusal for
`--usb-gadget`, with a named `pendingArtifactDTBs` test exemption for
exactly this in-flight state; (2) cut the artifacts release; (3) a
follow-up PR bumps `Version`, has the board consume the new DTB, flips
support back on, and removes the exemption.

The end-to-end round trip took two separate bench sessions to actually
prove, and the gap between them is worth internalizing for any future
gadget bring-up: a board with a correctly-bound ACM function looks
*identical* whether a real host is attached or the test cable is
power-only, because `gosd-init` has no shell to inspect
`/sys/class/udc/*/state` from and kernel messages are suppressed by
`quiet`. The first attempt got exactly that ambiguous result (device side
proven, host side saw nothing, could not tell why) and the fix — logging
`/sys/class/udc/*/state` transitions from the app itself — is now in
`examples/usbserial` for every future gadget bring-up to reuse, not just
this board's. With a real data-carrying host cable, the log line flipped
`not attached` → `configured`, the Mac enumerated vendor ID `0x0525` and
created `/dev/cu.usbmodem*`, and an echo round-trip passed (each line
arriving twice is `/dev/ttyGS0`'s own tty-layer echo plus the app's
write-back — expected, not a fault).

## Bench-rig gotcha: a card that "fails to boot" may just be in the wrong reader

Mid-session, a previously-good image + card combination started failing
identically after a physical reconnect, with SPL loading fine but its own
MMC driver unable to re-read the card (`mmc_load_image_raw_sector: mmc
block read error`, `Error: -38`). That split — BootROM's conservative read
path succeeds, U-Boot's own driver fails — looked like a marginal card,
and rhymes with a real card-specific issue seen on nanopi-zero2 (bean
`gosd-0abt`). It wasn't: during the physical reseat, the SD card had
ended up in the bench dock's own SD reader (`TS4 Card Reader`) rather than
the SDWire's mux (`USB3.0-CRW`) — macOS mounted it happily either way,
which is what made a correctly-routed rig look like a dying card. The
one-step diagnostic that settles this ambiguity: map each `/dev/diskN` to
its `Device / Media Name` and confirm it's the mux's reader, not any other
attached slot — `sdwire disk` reporting no block device (while macOS shows
one) is the tell that the card is elsewhere.

## Boot-time baseline

With the DRAM fix and the preboot-scan fix both applied: **6.98s mean**
(6.70–7.75s) power-on-adjacent (SPL banner) to `/app` running, split
U-Boot 4.50s (spread 0.03s across 5 power cycles) / kernel ~1.25s /
gosd-init→app ~0.06s — the fastest board in the fleet at time of writing.
The stock artifact prior to `gosd-uj4l`'s fix measured 10.38s (U-Boot
9.05s), comparable to rock-4se (9.21s) and nanopi-zero2 (10.33s).
