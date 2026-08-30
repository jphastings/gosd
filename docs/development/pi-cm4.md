# Developing for the Raspberry Pi CM4 (`pi-cm4`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-7676`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md and `build/boards/pi-cm4/kernel.fragment`'s own header comments;
this file is for things a future agent or developer would otherwise have to
rediscover by hand. If you're bringing this board up on a Turing Pi 2, read
[the shared Turing Pi 2 BMC notes](turing-pi-2-bmc.md) first — most of the
hard-won lessons from this board's bring-up turned out to be about the
baseboard, not the CM4 itself.

## This board's bring-up hardware may have onboard eMMC with someone else's
OS already on it

The specific CM4 module used for this board's bring-up turned out to have
onboard eMMC (despite being sourced as "Lite, no wireless" — those two
facts are independent; a module can lack wireless and still have eMMC) with
a working **Talos Linux** install already on it, and the Pi bootloader's
`BOOT_ORDER` prioritizes eMMC over SD. A freshly SD-flashed GoSD image will
silently never boot on a module in this state — the board just boots
whatever's already on the eMMC every time, with no indication anything is
wrong from the SD-flashing side. **Check for this before assuming an SD
flash "didn't take":** power the node, wait for network/console activity,
and see what's actually running before concluding the SD path is broken.

## Two DTS theories investigated and ruled out — don't re-check these

While chasing an unrelated console-silence problem (which turned out to be
a Turing-Pi-2 BMC issue, not a GoSD or DTS problem — see
[the shared Turing Pi 2 BMC notes](turing-pi-2-bmc.md)), two
plausible-looking DTS issues were investigated and definitively ruled out.
Recorded here so nobody re-spends the time:

- **`console=serial0` is correct, not backwards.** `bcm2711-rpi-cm4.dts`'s
  own `chosen.stdout-path = "serial1:115200n8"` looks suspicious at first
  glance (our `cmdline.txt` says `console=serial0`), but
  `bcm2711-rpi-ds.dtsi` — included later in the same file, so it wins —
  overrides the `aliases` node to `serial0 = &uart1` (the header-mapped
  UART) / `serial1 = &uart0` (BT), the *opposite* of the base
  `bcm283x.dtsi` mapping. `console=serial0` correctly resolves to `uart1`.
- **The empty `uart1_pins` pinctrl override is normal, not a bug.** The
  DTS's final `&uart1 { pinctrl-0 = <&uart1_pins>; }` points at a
  zero-length `{brcm,pins; brcm,function; brcm,pull;}` group, which looks
  like it could leave UART1 unmuxed. But `pi-3b`'s own DTS
  (`bcm2710-rpi-3-b-plus.dts`, at the same kernel commit) uses the
  *identical* empty group for its own mini-UART console, and pi-3b's
  console is proven working on real GoSD hardware. This is shared,
  harmless downstream-DTS boilerplate across the whole Pi family, not a
  CM4-specific defect.

## USB gadget mode is a genuine open question, not a known limitation

Unlike pi-3b (whose USB port is hard-wired through a hub with no UDC ever
possible) or the Turing RK1 (whose DTS binds its OTG-capable port to a
host-only XHCI driver), the CM4's dwc2 dual-role controller is a real,
kernel-compiled-in gadget-capable peripheral — there's no known hardware
reason gadget mode can't work here. `UsbGadgetSupport()` returns
`Supported: false` anyway, with a reason naming it *uncharacterized*, not
*unsupported* — nobody has verified whether Turing Pi 2's node carrier
actually routes the SoC's USB2 signals to an accessible port. If you get a
USB OTG dock connected to a CM4 node, testing this is a genuine
`Supported: true` candidate, not a wall to work around.

## Build/flash pipeline correctness — verified independently of any board
issue

When debugging a boot failure on this board, it's worth knowing the
build/flash pipeline itself was independently verified correct during
bring-up: read a flashed SD card back directly on macOS afterward (switch
the SDWire to host mode, `diskutil list` + mount) and every file matched
expectations exactly — right partition label, `cmdline.txt` byte-identical
to what `templates_test.go` asserts, correctly-sized `kernel8.img`/
`bcm2711-rpi-cm4.dtb`/`initramfs.cpio.zst`. If a board won't boot, that
level of read-back is a fast way to rule the pipeline in or out before
suspecting the kernel/DTS/BMC.
