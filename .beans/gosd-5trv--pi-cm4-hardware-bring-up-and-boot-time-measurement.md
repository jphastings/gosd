---
# gosd-5trv
title: 'Pi CM4: hardware bring-up and boot-time measurement'
status: in-progress
type: task
priority: normal
created_at: 2026-08-30T10:25:55Z
updated_at: 2026-08-30T18:46:31Z
parent: gosd-7676
---

## What

Bench: Turing Pi 2 (v2.4), CM4 in node 1, SDWire on its SD card, wired
Ethernet via the baseboard.

Verify the common core (bean pattern from every prior board bring-up):
build → flash (via SDWire) → boot → serial console → network up (DHCP) →
mDNS + HTTP reachable → power-cycle survival of /data (FAT32 default,
then ext4).

USB gadget mode is explicitly OUT of scope this round (epic's "?"
decision — no OTG dock connected). If a dock becomes available later,
characterizing it is a follow-up, not a blocker for closing this bean.


## Bench session (2026-08-30): eMMC surprise, BMC power/UART tooling notes, in progress

**Power/mode control: use the BMC's `tpi` CLI, not `sdwire power`.**
`sdwire power cycle` does NOT control a Turing Pi 2 node's actual power
rail — it appeared to succeed (`controlling power for device "bench"`)
but node 1 stayed off throughout. The correct mechanism is `tpi power
on/off/reset -n <1-4>` (1-indexed, matches the physical node labels)
against the BMC's own REST API. Also useful: `tpi uart get -n <N>` reads
a node's console directly through the BMC, no physical serial wiring
needed at all for basic verification — bypasses whatever's wrong with
the physical UART cable/TS4 dock path entirely.

**This CM4 module has onboard eMMC** (settles the "Lite, don't know if
it has eMMC" uncertainty from the epic) — and it already had **Talos
Linux** installed and running on it before this session touched
anything. `192.168.1.151`/`hello.local` responding for over an hour
during this session was a RED HERRING: a completely different,
already-running device (JP confirmed powering it off; uptime kept
climbing even after node 1 was powered off, proving it wasn't node 1).
Lesson: don't trust an mDNS name match on a shared network without
correlating it to the specific node (power the node off and confirm the
response drops).

**eMMC wipe + boot attempt (JP approved wiping):** zeroed the first
16MiB via `tpi advanced msd -n 1` (exposes eMMC as mass storage) +
`tpi flash -n 1 --image-path <zero.img> --skip-crc`. Board got stuck in
a repeating `Boot mode: RPIBOOT (03)` recovery loop afterward — the BMC's
msd-mode mechanism appears to latch the CM4 into USB-boot mode in a way
a plain `tpi power off`/`on` (even a genuinely cold ~20s cycle) does not
clear. `tpi reboot` (full BMC reboot, JP approved — costs node 4 power
too, which JP said was fine to lose) DID clear the loop.

Also flashed the real `hello` GoSD image directly onto the eMMC (a
pragmatic pivot from SD, since SDWire's USB control interface vanished
from the Mac mid-session — see below — and hasn't come back). Boot
chain reads `start4.elf` fine but `config.txt` comes back as 0 bytes
each time in the loop state; whether a straight `dd`-style raw write to
the exposed eMMC block device via `tpi flash` even lands in the right
place for the BOOTROM to find it (vs. a distinct eMMC hardware boot
partition needing explicit enablement) is UNVERIFIED — didn't get a
clean confirmed boot before pausing.

**Current blocker, session paused here:** after the BMC reboot cleared
the RPIBOOT loop, node 1's console went completely silent via
`tpi uart get` across two full power cycles (including one with a
genuinely cold ~20s power-off) — not even the BOOTROM banner that had
appeared reliably every time before. Either the BMC reboot disrupted its
own UART bridge for this node, or the board is genuinely hung. JP is
checking the physical setup (SDWire USB reseat, node 1 LEDs, possibly a
full physical power-cycle of the Turing Pi) before we resume.

**Recommendation for next session:** once SDWire's USB comes back,
prefer going straight back to the SD-boot path this board profile
actually implements, rather than continuing to chase eMMC-boot
semantics that are out of scope for `gosd-1tk8`'s design (SD-boot only,
eMMC deferred) — the eMMC detour cost real time without a confirmed
result and may need real Raspberry Pi eMMC-boot-partition research
(`mmc bootpart`-style enablement) neither attempted nor understood yet
if it's ever revisited.


## DTS investigation (2026-08-30): two hypotheses checked and ruled out

At JP's request, checked whether the eMMC boot silence (see above: console
+ network both go silent specifically when a real GoSD image, not zeros,
is on the eMMC) matches a known CM4 DTS issue.

**Hypothesis 1 — wrong serial alias (RK1-style bug), RULED OUT.**
`bcm2711-rpi-cm4.dts`'s `chosen.stdout-path = "serial1:115200n8"` looked
suspicious at first (our cmdline.txt says `console=serial0`), but
`bcm2711-rpi-ds.dtsi` (included later in the same file, so it wins)
overrides the aliases to `serial0 = &uart1` (the header UART) /
`serial1 = &uart0` (BT) — the OPPOSITE of the base bcm283x.dtsi mapping.
So `console=serial0` correctly resolves to uart1, the header UART. Not a
bug.

**Hypothesis 2 — empty `uart1_pins` pinctrl override leaves UART1
unmuxed, RULED OUT.** The CM4 DTS's final `&uart1 { pinctrl-0 =
<&uart1_pins>; }` does point at an empty `{brcm,pins; brcm,function;
brcm,pull;}` group, overriding the earlier populated `uart1_gpio14`
group. This looked like a real candidate — until checking pi-3b's own
DTS (`bcm2710-rpi-3-b-plus.dts`, at the same pinned commit): it uses the
IDENTICAL empty `uart1_pins` block for its own mini-UART console, and
pi-3b's console is proven working on real GoSD hardware
(COMPATIBILITY.md's "Maiden boot proven"). This is shared, harmless
downstream-DTS boilerplate across the whole Pi family, not a CM4-specific
defect.

**Net result: root cause of the eMMC-boot silence is still unknown.**
Both console-configuration theories checked out clean against the actual
device tree; the bug (if it is one) is somewhere else, or this may not
even be a GoSD/DTS issue at all — could be Turing Pi 2's own eMMC
mass-storage-mode write mechanism not landing content where the BootROM
reads it (original theory from earlier in this session), a genuinely
different board-specific hardware quirk, or something not yet
considered. JP asked to stop speculative fixing here rather than
continue guessing; picking this up again should start with either a
working console (SD path, once SDWire is un-wedged - bean gosd-l0xq) or
a way to read back the eMMC's actual content after a `tpi flash` write,
neither of which is available right now.


## Console access: root-caused two real BMC issues, still no signal from node 1

**Issue 1, SOLVED: the physical GPIO header carries no UART for CM4 nodes
at all.** Per the [official Turing Pi 2 spec](https://docs.turingpi.com/docs/turing-pi2-specs-and-io-ports):
"the UART header exposed through pins 8 and 10 is an additional UART in
the case of the Nvidia Jetson modules and these pins are not connected
in the case of the Raspberry Pi Compute Module 4." The physical TX/GND
wire JP moved onto node 1 (the RK1's old serial-console setup) was never
going to work for a CM4 node - it's not a wiring mistake, the pins are
simply unconnected on this hardware for this module type. The BMC's own
internal UART bridge (`tpi uart get -n <N>`) is the only console path
for a CM4 node on Turing Pi 2.

**Issue 2, SOLVED: `bmcd`'s UART buffer for node 1 got stuck serving a
frozen snapshot.** After an earlier `tpi reboot` (BMC-level reboot) in
this session, `tpi uart get -n 1` kept returning byte-for-byte identical
content (same hardware timer "stc" value) across many different power
cycles and even a full physical Turing Pi power-cycle - proving it was
serving a cached snapshot, not live data. SSH'd into the BMC
(`ssh root@turingpi.local`, same credentials as the REST API) and found
a single `bmcd` daemon (`/usr/bin/bmcd --config /etc/bmcd/config.yaml`)
handles power/uart/flash for every node - `/etc/init.d/S94bmcd restart`
(JP approved) cleared the frozen buffer.

**Still unresolved: node 1 produces ZERO bytes on its UART, confirmed
three independent ways after the bmcd restart** - `tpi uart get -n 1`,
a raw `cat /dev/ttyS1` directly on the BMC's own shell (bypassing bmcd
and the REST API entirely), and the (now-known-irrelevant) physical
header wire. Not even the BootROM banner appears, which printed
reliably and unconditionally in every earlier test this session
(including the zero-wiped-eMMC RPIBOOT loop). The other three
(unpowered) node ttys show 1 byte of line noise each during the same
window - expected floating-line behavior - while node 1's tty shows
literally nothing, not even noise, which is itself unusual.

Node-to-tty mapping was NOT assumed - confirmed empirically (all 4
ttyS1-4 captured simultaneously during a node-1-only power state) and
cross-checked against a public mapping quirk
([BMC-Firmware#181](https://github.com/turing-machines/BMC-Firmware/issues/181):
"/dev/ttyS4 corresponds to node 3", i.e. NOT a simple 1:1 index match) -
so this isn't a wrong-tty-guess issue either.

**What this rules out:** the RPIBOOT/eMMC-write-location theory from
earlier in this session no longer has support - the zero-image RPIBOOT
loop and current silence are BOTH being read through a now-confirmed-
reliable channel (raw tty + fresh bmcd), yet the same real GoSD image
(SD or eMMC) produces total silence, not a different-but-visible failure
mode. Whatever's happening now produces literally no BootROM output at
all, which the zero-image case never failed to produce. This might mean
something about how repeatedly toggling `tpi advanced msd`/`normal` this
session left node 1's boot-select or UART-mux GPIO strapping in a bad
state independent of card content - untested and unconfirmed.

**Independently confirmed correct, so not the cause:** the build/flash
pipeline itself. Read the SD card back directly on macOS after flashing
- partition label matches (`hello-boot`/`tmp-di-boot`+`tmp-di-data`),
every expected file present at the right sizes (config.txt, cmdline.txt
byte-identical to what templates_test.go expects, kernel8.img,
bcm2711-rpi-cm4.dtb, initramfs.cpio.zst), and a throwaway diagnostic app
(dumps /proc/cmdline, /proc/consoles, /dev/kmsg, etc. to /data/diag.txt)
built and flashed cleanly - but diag.txt was never written, consistent
with the failure happening before gosd-init's own app-launch stage, not
after.

**Recommendation for next session:** try a full, genuinely cold
power-cycle of the WHOLE Turing Pi 2 (mains unplugged, not just
tpi power off/on or tpi reboot) combined with `tpi advanced normal -n 1`
run explicitly first (in case a stale msd/rpiboot GPIO strap is the
cause) before the cold cycle, then retest with `tpi uart get -n 1`
immediately - if the BootROM banner reappears, the msd-mode-strap theory
is confirmed and future eMMC bring-up sessions should always end on
`tpi advanced normal` before power-cycling. If it's still silent after a
genuine cold cycle, this may be a hardware fault on this specific CM4
unit or Turing Pi 2 node 1 slot, not a GoSD or BMC firmware issue, and
would need a different CM4 module or node slot to isolate.
