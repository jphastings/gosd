# Developing for the NanoPi Zero2 (`nanopi-zero2`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-cwjf`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md and COMPATIBILITY.md's board notes (the 30-pin FPC connector, no
working USB gadget in the stock kernel); this file is for things a future
agent or developer would otherwise have to rediscover by hand.

## U-Boot is pinned to a release candidate, not a stable release

`build/boards/nanopi-zero2/uboot/build.sh` pins `UBOOT_TAG="v2026.07-rc5"`.
`configs/nanopi-zero2-rk3528_defconfig` (this board's dedicated defconfig,
with `CONFIG_USB_GADGET`/Rockusb already enabled) only exists from the
v2026.07 cycle onward and wasn't in any prior tagged release. The original
gate (bean `gosd-vcae`) said to wait for the final v2026.07 tag; JP amended
that 2026-07-07 to pin the latest available `-rc` instead so the board would
be hardware-testable sooner — an rc tag is still a fixed, reproducible git
ref, just not the LTS/stable line the project otherwise prefers. Re-pinning
to the final `v2026.07` release (and rebuilding/re-releasing artifacts) is
still an open follow-up on bean `gosd-f39b` — check `build.sh`'s `UBOOT_TAG`
before assuming it's been done.

## Debug UART is on a separate 8-pin header, not the 30-pin FPC connector

FriendlyElec breaks the console out on its own 2.54mm 8-pin header (pin 3 =
TX, pin 5 = RX, pins 1/6/8 = GND, pin 7 = VCC_3V3), entirely separate from the
30-pin FPC GPIO connector. Baud is 1,500,000 (FriendlyElec's usual
convention), confirmed both in the DT (`stdout-path = "serial0:1500000n8"`)
and on real hardware.

FriendlyElec's own wiki labels these pins `UART2DBG_TX`/`UART2DBG_RX` —
that's wrong, or at least misleading (bean `gosd-odp7` suspects recycled
RK3399-era boilerplate). The mainline DT's `/aliases` node has exactly one
serial alias (`serial0 = &uart0`), so `console=ttyS0` is correct and is what
actually appears on that header; there is no separate "UART2" involved.
Wire TX-only and attach the adapter's TXD *after* power-on — same
back-powering caution as other boards.

## USB is completely absent — not merely "no gadget", no controller at all

COMPATIBILITY.md's `[^nanopi-usb]` footnote already says this board has no
USB at all, host or gadget, until a future kernel bump. Worth knowing if you
go looking for *why*: `rk3528.dtsi` has no USB controller DT node whatsoever
at the pinned kernel tag (`v6.18.37`) — not disabled, not misconfigured,
simply absent — verified directly against the tag's source, including
`phy-rockchip-inno-usb2.c`'s `of_device_id` table (no `rk3528` entry). The
node (`usb_host0_xhci`, a genuine dwc3 controller) and the board-level
USB-enable commit only exist on mainline `master`/`v7.2-rc4` and aren't in
any numbered kernel release (commits `5f3ae9b12a6c` and `ff660109f412`, both
2026-06-02). Because the arm64 defconfig baseline already builds in the
generic USB/dwc3/gadget-configfs stack regardless of board, the compiled
kernel binary contains those symbols `=y` — they're just inert, with nothing
in the device tree to bind to. `build/boards/nanopi-zero2/kernel/kernel-
fragment.config`'s own comment block records this in detail.

The unlock plan is fully pre-baked for whoever picks it up (bean `gosd-woox`,
item W1 — this superseded the original tracking bean `gosd-36yy`, scrapped
2026-08-21 as part of a consolidation, not abandoned): once a numbered
release ships those two commits, promote the fragment's USB symbols to
required, and add a new DTS patch forcing `dr_mode = "peripheral"` on
`&usb_host0_xhci`. That's a deliberate choice even though this board's
Type-C port is a genuine OTG port (unlike rock-4se, where peripheral-only was
forced by elimination) — because the board *also* has a separate, dedicated
USB 2.0 Type-A host port, dedicating the Type-C DWC3 controller to gadget
mode costs no host capability and gives the configfs gadget stack a
deterministic UDC at boot rather than depending on extcon/ID-pin
role-switching. One thing explicitly left unchecked for that future work:
whether `phy-rockchip-inno-usb2.c` gains an `rk3528` `of_device_id` entry
alongside the new node — verify before trusting `CONFIG_PHY_ROCKCHIP_INNO_USB2`.

## "0 bootflows" on a card that SPL itself booted from means check the card's reported capacity

A field report (bean `gosd-0abt`, still open) hit U-Boot SPL booting cleanly
from an SD card, then U-Boot proper's bootstd scan silently reporting
`(0 bootflows, 0 valid)` on the exact same card — kernel never loads, no
error printed anywhere because autoboot swallows per-bootdev errors. Root
cause, confirmed on the bench: **the specific SD card**, not a gosd
regression — a replacement card booted the identical image fine. The failing
card deterministically misnegotiated its own capacity with U-Boot's
`dw_mshc` init (`mmc info` reported `High Capacity: No, Capacity: 30.6 MiB`
against a real multi-GB card, sticky across `mmc rescan`), while the same
card read correctly via SPL's more conservative init moments earlier in the
same power-on, and correctly in a macOS USB reader. Below ~30MiB everything
still worked (MBR, BPB, root directory), which is why SPL and the FAT probe
both looked fine; only `/extlinux/extlinux.conf` — pushed out past 30MiB by
the kernel `Image` occupying the preceding clusters — became unreachable.

If this recurs: interrupt autoboot (needs serial TX wired, not just RX) and
run `bootflow scan -l` (the `-l` flag prints the per-bootdev errors autoboot
otherwise swallows) plus `mmc info` / `mmc dev 1; mmc info` — a
`High Capacity: No` result on a card that is obviously not actually a
~30MiB SDSC card is the signature. `CONFIG_CMD_MMC_REG` (raw OCR/CSD access)
is **not** compiled into this board's U-Boot, which blocked deeper forensics
in the original investigation — enabling it is a queued follow-up if this
needs re-diagnosing. This was chased down a UHS-voltage-switch theory first
(the control DTB does carry `sd-uhs-sdr104`/tuning); that theory was directly
refuted at the prompt (`mmc info` showed the card never reached a UHS path)
and is recorded in the bean only so nobody re-spends the time re-deriving it.

## Other bring-up notes

- **eMMC can shadow the SD boot partition.** With eMMC fitted, gosd-init's
  name-ordered boot-partition probe tried the eMMC's (non-FAT) partition 1
  before falling through to the SD card — harmless here only because that
  partition wasn't valid FAT (bean `gosd-pcwl`, fixed).
- **The kernel fragment is nearly identical to Radxa Zero 3E's.** Diffing the
  two boards' generated `kernel.config`s, the only option-line difference is
  `CONFIG_MOTORCOMM_PHY` (needed for a Radxa board-revision variant, not
  fitted here — this board's GbE PHY is a Realtek RTL8211F, `CONFIG_REALTEK_PHY`).
- **Boot-time baseline** (SPL banner to app exec, direct card slot, `hello`
  example, eMMC fitted): 10.33s ± 0.03s across 6 power cycles, plus one
  unplanned mid-boot power-loss recovery that also came back clean (bean
  `gosd-odp7`). No TPL banner is visible to anchor an earlier start point —
  the rkbin DDR-init blob doesn't print one.
- **The bench SDWire rig was flaky for this board's first session** — later
  isolated to the rig itself (rock-4se, already proven working via physical
  card swaps, was equally silent through the same rig) rather than anything
  board- or image-specific. If a fresh bring-up hits total serial silence
  through SDWire, try a direct card-slot flash before suspecting the board.
