---
# gosd-3io0
title: 'cubie-a5e USB gadget can''t work: ehci0/ohci0 steal the OTG phy at boot'
status: todo
type: bug
priority: normal
created_at: 2026-08-17T06:05:25Z
updated_at: 2026-08-17T08:08:17Z
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
- [~] If option 1: write the DTS patch, rebuild the kernel, re-run this probe,
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



## Bench: the approach works (2026-08-17, hand-patched DTB)

Before committing to a kernel rebuild, the fix was tested the cheap way: the
shipped DTB decompiled, ehci0/ohci0 set to `status = "disabled"`, recompiled
with dtc, and dropped onto the boot partition — no kernel build needed, since
extlinux just loads a file.

| | stock DTB | patched DTB |
|---|---|---|
| `phy phy-4100400.phy.0: Changing dr_mode to 1` | present | **gone** |
| `ehci-platform 4101000.usb` (ehci0, on phy 0) | probes | **absent** |
| `ehci-platform 4200000.usb` (the Type-A port) | probes | **still probes** |

So the host controllers no longer take the phy, and the USB 3.0 Type-A port —
the one real regression risk — is unaffected. The board otherwise boots
normally: DRAM, kernel, gosd-init, `/app`, DHCP lease, mDNS and NTP all fine,
and `gadget.Apply` still succeeds with the gadget bound to the UDC.

**Not proven: enumeration.** The bench has moved back to Meross power, so the
USB-C now carries power from a PSU rather than a data link to the Mac, and
there is no host to enumerate against. `state` stays "not attached", which is
the expected reading with no host attached and says nothing either way about
the fix. Proving the round-trip needs the USB-C wired to the Mac again with a
data cable, which is what the follow-up PR (support back to ✅) must wait for.

That splits the evidence cleanly: the mechanism (phy freed, Type-A intact) is
proven now, and belongs to the PR that ships the DTB and corrects the claim;
the end-to-end round-trip is the one thing the ✅ restoration turns on.



## The real built DTB, verified on hardware (2026-08-17)

`gosd build-kernel --board cubie-a5e` with the patch produced both blobs, so
the variant DTS compiles as part of a normal build:

| DTB | ehci0 | ohci0 | ehci1 (Type-A) | usb_otg |
|---|---|---|---|---|
| `sun55i-a527-cubie-a5e.dtb` | okay | okay | okay | okay |
| `sun55i-a527-cubie-a5e-gadget.dtb` | **disabled** | **disabled** | okay | okay |

Booted on the board (image built from the build's own artifacts, not the
hand-patched blob): `ehci0`/`ohci0` never probe, no `Changing dr_mode` line,
`ehci-platform 4200000` (the Type-A port) still comes up, `gadget.Apply`
succeeds, and the board reaches `/app` with a DHCP lease. Same result as the
hand-patched proof, now from the artifact we would actually ship.

Enumeration remains the one unproven step — the bench is back on Meross power,
so the USB-C carries power rather than a data link to the Mac. It is the gate
on restoring ✅, not on shipping the DTB.
