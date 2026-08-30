# Developing for the Raspberry Pi Zero 2W (`pi-zero-2w`)

Bench/bring-up knowledge from the initial board-support work (epic
`gosd-vmgw`) that isn't captured elsewhere. Locked design decisions live in
CLAUDE.md; this file is for things a future agent or developer would
otherwise have to rediscover by hand.

## A silent, healthy-looking boot can mean "no device tree" — check the FAT partition before suspecting the kernel

The first hardware boot was completely silent on serial — no console output
at all — but with a normal-looking blinking ACT LED, the kind of signal that
reads as "board is fine, something minor is off" rather than "boot never
really started." Root cause (`gosd-f59k`): the assembled boot partition
never carried `bcm2710-rpi-zero-2-w.dtb` at all — `internal/boards/pizero2w`
had simply never declared a DTB artifact, unlike `pizerow`, which already
did. With no device tree the kernel has no UART, so it hangs before any
console output can appear. Diagnosed by inspecting the flashed card directly
and confirmed by hand-copying a DTB from the artifact cache onto GOSD-BOOT —
no reflash needed, boot proceeded immediately. Fixed by adding the DTB to
the board profile's `Artifacts()`/`BootFiles()` (mirroring `pizerow`'s
existing pattern), with a boot-partition-completeness assertion added to the
build integration test so the omission can't recur silently. The technique
worth keeping for any future silent-boot mystery on this board: **the boot
FAT partition can be inspected and hand-edited without a reflash** — it's
the fastest way to confirm whether a card actually received everything it
needed before suspecting the kernel or DTS.

## The WiFi "associate then drop" loop was neither — the bisection technique matters more than the bug

`gosd-m9dj`'s first WiFi join looped forever, logging "associated with
`<AP>`" followed almost immediately by "lost its WiFi association;
reconnecting" — read at the time as a real 4-way-handshake failure or an
AP-side deauth. Several plausible leads were run down and eliminated by the
desk research in `gosd-anyp`: PSK derivation (checked against the IEEE
802.11 test vector), WiFi firmware blob completeness/version, and a Pi
downstream-vs-mainline kernel tree delta (the pinned tree already IS the
downstream one; the relevant `brcm80211` code is byte-identical to
mainline). A WPA3-transition/PMF theory on the test AP (a Netgear RAXE300
tri-band 6E router) looked the most convincing and was still wrong — it
survived a hotspot forced into WPA2-only mode too, which should have ruled
out the AP sooner than it did.

The move that actually cracked it: **booting a known-good OS on the exact
same board against the exact same AP.** A gokrazy image joined the failing
AP on the first attempt, which isolates hardware/AP/environment entirely and
convicts "our software stack" as a whole. The follow-up generalizes well
beyond WiFi: **port the suspect code, verbatim, into that known-good image
as a side-by-side probe** — a byte-for-byte copy of the connect path
(`wifiup-probe`) run inside the working gokrazy environment reproduced the
loop there too, which narrowed the fault down to the *code itself* rather
than the Pi kernel build or config trim, closing off a whole axis of
Pi-specific suspicion in a single boot.

Root cause (the generic lesson is in CLAUDE.md's netlink section): the
CONNECT netlink message was missing `netlink.Request`, so the kernel
silently skipped it while still returning a success ack — every join
attempt was a no-op that never reached the firmware. The misleading
"associated with" log line — printed on the CONNECT ack, not on real
association — is what made a silent no-op look like a handshake/deauth loop
for two bench days. Fixed alongside the netlink flag: honest logging
("connect accepted; awaiting association" instead of "associated"), plus
deauth reason-code logging that would have discriminated the real failure
mode immediately had it existed from the start.

One curiosity survived the whole investigation unexplained, and is believed
benign: the WiFi interface enumerates as `wlan2` (not `wlan0`) on every GoSD
boot on this board, even a fully healthy one — the same hardware running
gokrazy enumerates `wlan0`. Nothing has ever been traced back to it; if it
starts mattering, the next step is watching `dmesg` for brcmfmac
reprobe/reset activity early in boot.

## Boot timings: ~25s power-to-HTTP over WiFi is the accepted baseline, not a regression

Power-to-app-running is fast on this board (~9s: ~2-3s Pi GPU firmware +
~6.1s to gosd-init + ~0.4s to the app starting), but power-to-**HTTP
reachable** is ~25s wall-clock, because WiFi association, DHCP, and the mDNS
announce all happen only after the app is already running. JP ruled
(`gosd-m9dj`, 2026-08-21) that ~25s is accepted as the WiFi-path reality
rather than something to keep chasing, and rescoped the project's original
"<15s" (stretch <10s) power-to-HTTP target to wired boards only (e.g.
rock-4se's ~9.2s wired). Don't read a ~25s WiFi boot-to-HTTP figure on this
board as a regression.

## Proving a dead console is a wiring fault, not a software one, with no console to test with

This board's kernel shares the exact same `CONFIG_SERIAL_8250_RUNTIME_UARTS=0`
defconfig default that killed [pi-zero-w's console outright](pi-zero-w.md) —
a real, initially-alarming inconsistency once noticed. But this board's
console had already gone silent on the bench for an unrelated reason (see
below), so the two symptoms briefly looked like the same bug. Settling
which one was real needed a diagnostic technique worth reusing whenever a
board's serial line dies and standard filesystem inspection is unavailable
too: **flash a purpose-built app whose `/data` is FAT32** (so a plain macOS
mount can read its output) **that dumps the kernel's own view of its own
state** — `/proc/cmdline`, `/proc/consoles`, and a `/dev/kmsg` drain (which
retains everything despite `quiet`) — to a file on that partition.

That app proved, entirely without a working serial link: the Pi firmware
*does* inject `8250.nr_uarts=1` into the cmdline on this board (so the
RUNTIME_UARTS=0 defconfig default is harmless here, unlike pi-zero-w), the
`ttyS0` console registers and is marked preferred in `/proc/consoles`, and a
direct `/dev/gpiomem` register read of `GPFSEL1` confirmed GPIO14/15 are
correctly muxed to the mini-UART's ALT5 function — every software layer
checked out. That leaves only the physical link, which is exactly what made
the next section's "reseat the jumpers" fix the right call rather than a
guess: with every software layer positively confirmed, a dead console can
only be the wire, the adapter, or a pin-labeling mixup. Bean `gosd-ehkt`
(closed as "not a bug" on this board, but the technique is the lasting
value — it's the same shape later reused for a completely different board's
bring-up when its BMC-mediated console turned out to be unreliable too).

## Bench gotchas from the hardware bring-up session

- **Reseat the GPIO14/15 serial jumpers if the console suddenly goes dead
  mid-session.** The bench serial link died partway through a power-cycle
  run with no other symptom (0-byte captures on a previously-working
  setup) — a wiring issue, not a software regression.
- One post-test boot took ~131s to become HTTP-reachable with no serial
  coverage of that particular run — never explained, never reproduced.
  Recorded rather than chased; worth a closer look if it recurs.
- The 5/5 power-cycle survival result was confirmed at the network level
  only for part of that run, because serial coverage was lost partway
  through (see above) — if a future session needs serial-level power-cycle
  evidence, confirm the serial link is actually capturing before trusting
  the result.
