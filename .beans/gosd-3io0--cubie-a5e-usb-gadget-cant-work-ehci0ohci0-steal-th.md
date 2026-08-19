---
# gosd-3io0
title: 'cubie-a5e USB gadget can''t work: ehci0/ohci0 steal the OTG phy at boot'
status: completed
type: bug
created_at: 2026-08-17T06:05:25Z
updated_at: 2026-08-17T06:05:25Z
parent: gosd-h1wv
---

Bench-proven 2026-08-17 (bean gosd-6pfn's gadget todo): `gosd build --usb-gadget`
produces a working-looking image on cubie-a5e, but the board can NEVER enumerate
as a USB device at the pinned artifacts. Our COMPATIBILITY.md ✅ and
`internal/boards/cubiea5e`'s `UsbGadgetSupport{Supported: true}` are both wrong.

## What the board does

`examples/usbserial` starts cleanly and gosd's own layer is entirely correct —
a purpose-built probe app confirms the peripheral controller is registered, our
gadget is bound to it, and the ACM endpoint exists:

```
PROBE: t=030s udc=musb-hdrc.2.auto state="not attached" speed="UNKNOWN" function="gosd"
PROBE: /dev/ttyGS0 exists
```

`state="not attached"` means the controller sees no host on the wire. The Mac
sees nothing at all: no `0x0525` NetChip device, no new `/dev/cu.usbmodem*`,
USB tree byte-identical to before the board was plugged in.

## Root cause, from the kernel's own log

The OTG phy is switched to HOST mode during boot, before the gadget ever has a
chance:

```
[0.866933] phy phy-4100400.phy.0: Changing dr_mode to 1     <- 1 = USB_DR_MODE_HOST
[0.866963] ehci-platform 4101000.usb: EHCI Host Controller  <- ehci0 probing, 30us later
```

`ehci0` is `usb@4101000`, and in `sun55i-a523.dtsi` it declares
`phys = <&usbphy 0>` — **the same phy index 0 that `usb_otg` (MUSB) uses**.
sunxi's phy0 is dual-route: the port goes either to MUSB (peripheral) or to
EHCI0/OHCI0 (host). The board DTS
(`sun55i-a527-cubie-a5e.dts`, v6.18.37) enables BOTH:

```
&usb_otg { dr_mode = "peripheral"; status = "okay"; };
&ehci0  { status = "okay"; };
&ohci0  { status = "okay"; };
```

so ehci0's probe calls `phy_set_mode(PHY_MODE_USB_HOST)` and wins. MUSB keeps
its UDC — which is why `Apply()` succeeds and `/dev/ttyGS0` appears — but the
phy is routed away from it, so D+ is never pulled up.

Normally an OTG port arbitrates via ID/VBUS detection, but this board has none,
and the DTS says so explicitly:

> The schematic describes USB0_ID (PL10), measuring VBUS_5V, which looks to be
> always on. Also there is USB-VBUSDET (PL2), which is measuring the same
> VBUS_5V. ... none of them seem to make any sense in relation to detecting USB
> devices ... The AXP717C provides proper USB-C CC pin functionality, but the
> PMIC is not connected to those pins of the USB-C connector.

With no detection there is no arbitration, so probe order decides, and host wins
every boot.

## Ruled out (don't re-test these)

- **Cable**: reproduced with a known-good data cable.
- **Port**: a ThingM blink(1) enumerates through the same TS4 port.
- **Wrong connector**: Radxa's spec says "1x USB 2.0 Type-C OTG with 5V power" —
  one Type-C, doing power and OTG; the Type-A is "1x USB 3.0 Type-A HOST".
- **Our kernel config**: `CONFIG_USB_MUSB_HDRC/DUAL_ROLE/SUNXI` are all `=y`,
  consistent with the UDC existing and accepting the gadget.
- **gosd's gadget layer**: `gadget.Apply()` materialized configfs and bound the
  UDC; nothing in `gadget/` is implicated.

## Fix options

1. **DTS patch disabling ehci0/ohci0** (`build/boards/cubie-a5e/kernel/patches/`,
   the non-Pi convention). phy0 then stays with MUSB and peripheral mode works.
   Cost: the USB-C port loses host capability — acceptable, since the Type-A
   port is served by ehci1/ohci1 and gosd never boots from the USB-C. Needs a
   kernel rebuild and an artifacts release (tag-first, bump-second).
2. **Leave host mode and drop the gadget claim** — cheapest, and honest, but
   loses a feature the epic scoped as in-scope.

Option 1 is a build-time either/or, not a runtime choice: without detection
hardware the port cannot be both. Whichever way it goes, it should be one
board-level decision recorded here.

## Todos

- [x] Correct the false support claim NOW, ahead of any kernel work:
      COMPATIBILITY.md's ✅ and `cubiea5e.UsbGadgetSupport` — `gosd build
      --usb-gadget --board cubie-a5e` must refuse with an actionable error
      rather than shipping an image that cannot work
- [ ] Decide between the two fix options above (JP)
- [x] If option 1: write the DTS patch, rebuild the kernel, re-run this probe,
      and confirm the Mac enumerates `/dev/cu.usbmodem*` with an echo round-trip
- [ ] Re-check whether the Type-A host port still works after any such patch


## Direction taken (2026-08-17)

Option 1, but as a **variant DTB rather than a change to the stock one**. The
stock DTS deliberately caters for powering the board from the GPIO 5V pins,
where the USB-C legitimately becomes a host port, so disabling ehci0/ohci0
outright would quietly remove that. Instead the kernel build emits both, and
`--usb-gadget` decides which one an image carries — the same shape as the Pi
boards shipping `dwc2.dtbo` only when the flag asks for it.

This cannot land in one PR, because artifact resolution is eager over every
ref a board lists: naming a DTB the pinned release doesn't carry would fail
EVERY cubie-a5e build, not just gadget ones. So it splits along the existing
tag-first/bump-second seam:

1. **This PR** — the DTS patch, the kernelspec output, and the honest interim:
   `--usb-gadget` refuses for this board and COMPATIBILITY.md says ❌ with the
   reason. `internal/kernelspec`'s outputs-vs-artifacts test gains a
   `pendingArtifactDTBs` exemption naming exactly this in-flight state.
2. **Artifacts release** cut from it.
3. **Follow-up PR** — `Version` bump, the board consuming the new DTB, support
   flipped back to ✅, and the exemption removed.


## Consuming the DTB (2026-08-17)

With `internal/artifacts.Version` now at v0.10.2 — which carries
`sun55i-a527-cubie-a5e-gadget.dtb` — the board can select it, so this is the
other half: `Artifacts()` lists it, `BootFiles` picks between stock and
variant on `cfg.UsbGadget`, `extlinux.conf` names whichever shipped, and
`kernelspec`'s `pendingArtifactDTBs` exemption is gone (its whole reason was
the release window, now closed).

COMPATIBILITY.md goes to **⚠️, not ✅**: the device tree is proven on hardware
to keep the phy with the peripheral controller and to leave the Type-A port
working, but a full enumeration round-trip against a host has never run — the
bench's USB-C has carried power, not data, since the day this was found. That
last step is the only thing between ⚠️ and ✅.

## Bench attempt at the enumeration round-trip (2026-08-19) — device side
## good, host side saw nothing

Built `examples/usbserial` for cubie-a5e with `--usb-gadget` from this branch
and flashed it via the SDWire. The image is right: only
`sun55i-a527-cubie-a5e-gadget.dtb` ships (the stock DTB is absent) and
`extlinux.conf` loads it, confirmed by reading the built image back.

**The device side works, on every boot.** U-Boot retrieves the gadget DTB,
and the app reaches:

```
gosd usbserial: gadget applied, waiting for /dev/ttyGS0
gosd usbserial: echoing lines over /dev/ttyGS0
```

Opening `/dev/ttyGS0` means the ACM function bound to a UDC, which can only
happen with MUSB present and in peripheral mode — so the variant DTB is doing
its job at runtime, not just on paper.

**The host side saw nothing at all.** Snapshotting the Mac's USB tree
(`ioreg -p IOUSB`) before and after a power cycle gave a byte-identical 35
entries: no new device, nothing with the gadget's vendor ID 0x0525, and no
`/dev/cu.usbmodem*` node at any point. Not a failed or partial enumeration —
no enumeration attempt reached the host.

**This does not distinguish a cable from a bug**, and it should not be
recorded as either. The board's own view of whether a host is attached was
not observable: kernel messages are suppressed by `quiet`, and gosd-init has
no shell, so `/sys/class/udc/*/state` — the one file that settles it — could
not be read.

### The discriminator, for whoever picks this up

Log `/sys/class/udc/*/state` from the app. It reads `not attached` with no
host or no data path, and `configured` once a host has enumerated it. That
single line separates "the bench USB-C is still power-only" from "the gadget
does not enumerate", which is the whole remaining question. Worth adding to
`examples/usbserial` rather than a throwaway, since every future gadget
bring-up on any board hits this same blind spot.

**COMPATIBILITY.md stays ⚠️.** Nothing here justifies ✅, and nothing here
contradicts the DTB work either — this PR remains correct and a prerequisite
either way.

## Round-trip CONFIRMED (2026-08-19) — ⚠️ → ✅

The USB-C was then wired to a host with a data cable, and the controller's own
state file settled it instantly:

```
gosd usbserial: USB controller state: musb-hdrc.2.auto=not attached
gosd usbserial: USB controller state: musb-hdrc.2.auto=configured
```

`configured` means the host completed enumeration. The host agreed: a device
with the gadget's vendor ID `0x0525` appeared in the USB tree and macOS
created `/dev/cu.usbmodem1111301`.

Echo round-trip over that node **passes**: four lines sent, including a
60-character one, each returned intact. Every line comes back twice, which is
expected rather than a fault — `/dev/ttyGS0` is a terminal, so the tty's own
echo arrives alongside the app's write-back. (A first attempt appeared to
fail with replies lagging one message behind; that was the test not draining
between sends, not the device.)

So the full chain is proven on hardware: variant DTB ships and loads → MUSB
present and in peripheral mode → ACM binds → host enumerates → data flows
both ways. COMPATIBILITY.md goes to ✅ and the footnote records the
verification rather than the caveat.

### The blind spot this closed, kept

The earlier attempt could not tell "the bench cable carries power only" from
"the gadget does not enumerate", because a board with a bound ACM function
looks identical either way and has no shell to inspect itself with. That gap
is now closed in `examples/usbserial`, which reports every USB controller
state change to the console. It cost one boot to go from an unanswerable
question to a definite answer, and every future gadget bring-up on any board
would otherwise hit the same wall.
