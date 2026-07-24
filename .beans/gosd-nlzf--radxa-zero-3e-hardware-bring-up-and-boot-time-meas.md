---
# gosd-nlzf
title: Radxa Zero 3E hardware bring-up and boot-time measurement
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:02:28Z
updated_at: 2026-07-24T15:53:32Z
parent: gosd-v370
blocked_by:
    - gosd-c7tk
    - gosd-gbsz
    - gosd-3zrc
    - gosd-m0vj
    - gosd-vtce
---

On-hardware validation on the Radxa Zero 3E. Requires USB-UART on the debug pins (1500000n8) and an Ethernet cable to a DHCP network.

- [ ] Build examples/hello image, flash, capture full serial boot log into this bean (TPL/SPL → U-Boot → kernel → gosd-init → app)
- [ ] App reachable over Ethernet via HTTP; record the DHCP lease appearing in logs
- [ ] Measure and record: power-to-U-Boot, U-Boot handoff time, kernel-to-init, power-to-HTTP-reachable
- [ ] If U-Boot adds more than ~2s, note follow-up options in this bean (SPL falcon mode, CONFIG_BOOTDELAY=-2) and file a follow-up bean
- [ ] 5× power-cycle survival test
- [ ] File bug beans for every deviation; list them here

## Acceptance
Boot log + timings recorded here; power-to-HTTP under 15s (stretch 10s); 5/5 power cycles.

### Bring-up session 1 (2026-07-24) — boots, with a serial-adapter discovery

**GoSD boots on the Radxa Zero 3E** (two separate units tested): full chain to
app + DHCP (192.168.1.233) + mDNS (usbwebsite.local resolves & pings from
macOS). First hardware validation of the radxa-zero-3e artifacts (RKNS path
shared with nanopi-zero2, previously proven there). JP's unit(s) have NO
onboard eMMC — usbwebsite correctly reports this and exits (its restart loop
runs indefinitely; see follow-up notes).

**The serial saga — root cause found (~2h of debugging):** at the standard
1,500,000 baud, output arrives as deterministic garble on a CP2102N adapter:
right byte count, right burst cadence (gosd-init restart backoff 1s→2s→4s→8s
→10s cap was identifiable from garble alone), but unreadable — received bytes
skew high-bits-set/low-bits-clear = slow RISING edges (RC-style) as seen by
the adapter. Both Zero 3E units, multiple OSes (GoSD + Armbian), proven-good
wires/adapter/capture (loopback-verified at 1.5M). The SAME adapter reads
rock-4se (RK3399) and nanopi-zero2 (RK3528) perfectly at 1.5M — this is
RK3566-family TX drive vs CP210x input, and Radxa's own serial doc warns
'CP210X and PL2303x some products have baud rate limitations' and recommends
CH340 cables (they also ship a patched picocom brew tap for macOS 1.5M).

**Workaround proven:** edit console=ttyS2,115200n8 in extlinux.conf on the
GOSD-BOOT FAT partition (no reflash needed) — kernel-onward output is 100%
readable at 115200. U-Boot's own output stays at its compiled-in 1.5M
(unreadable on this adapter; a CH340 is on the shopping list for full U-Boot
visibility).

**Debugging red herrings recorded for posterity:** a stale background capture
holding the port split bytes between readers (now in the serial gotchas
memory); wrong-pin/180°-header and preloaded-eMMC theories disproven (JP's
colour-coded header pins were right; SD-pull test showed heartbeat tracks our
SD). The boot-cadence-from-garble trick (matching restart backoff timing) is
a genuinely useful diagnostic: it proves the software stack end-to-end before
a single byte decodes.

Remaining: boot-time baseline + power-cycle survival (needs readable-or-CH340
serial for timestamps), gadget test via usbserial (this board HAS a UDC),
GbE/HTTP with an app that serves (hello), I2C/SPI/GPIO items.
