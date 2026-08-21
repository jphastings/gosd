---
# gosd-sc9w
title: 'v0.1 — Hello GoSD: both boards boot a Go app with network'
status: completed
type: milestone
priority: normal
created_at: 2026-07-02T20:46:56Z
updated_at: 2026-08-21T04:48:05Z
---

MVP milestone. Definition of done:

- `gosd build ./examples/hello --board=pi-zero-2w` and `--board=radxa-zero-3e` each produce a flashable .img on a plain macOS/Linux machine with only Go installed (no root, no Docker at build time).
- Flashing the image (dd or Raspberry Pi Imager, no customization) boots the board into the example Go app in under 10 seconds from power-on.
- Pi Zero 2W joins WiFi from credentials baked into the image at build time (end-user Imager provisioning is v0.2); Radxa Zero 3E gets a DHCP lease over Ethernet.
- Example app serves HTTP on :80 and logs to serial console.

Both boards are developed in parallel — neither is 'the port'.


## Ruling: the boot-time target is re-scoped to wired paths (JP, 2026-08-21)

The definition of done above says "boots the board into the example Go app in
under 10 seconds from power-on". That target predates any hardware, and
gosd-m9dj's bring-up asked JP to rule on it once real numbers existed. The
ruling: **~25s power-to-HTTP over WiFi is accepted as the reality, and the
target applies to wired paths.** No boot-time-optimization bean is filed.

**The distinction that matters, and the one to carry into any future
statement of this target: "the app is running" and "the app is reachable at
`hostname.local`" are different numbers.** The first is under 10 seconds. The
second is not, on WiFi.

Measured on real hardware (bean gosd-m9dj, session 2, 2026-07-25):

| Measurement | Board | Figure |
|---|---|---|
| Pi GPU firmware, before kernel time zero | pi-zero-2w | ~2-3s |
| kernel start → gosd-init | pi-zero-2w | ~6.1s |
| gosd-init → app running | pi-zero-2w | ~0.4s |
| **power → app running** | pi-zero-2w | **~9s** |
| **power → HTTP reachable, WiFi** | pi-zero-2w | **~25s** (5s-granularity poller) |
| **power → HTTP reachable, wired** | rock-4se | **~9.2s** |

The app itself is up at ~9s, comfortably inside the target. What the WiFi path
adds on top — association, DHCP and mDNS announcement — all happens *after*
the app is already serving, and none of it is GoSD being slow: it is what
joining a wireless network costs. The wired boards hit the original number
without special effort (rock-4se ~9.2s power-to-HTTP), which is why the target
belongs to them.

Followed through in the README, which promised "under 5 seconds, WiFi
included" — false on both halves. It now states about 10 seconds to the app
and to a wired `hostname.local`, and ~25s over WiFi, with the reason.
## Summary of Changes

Closed retrospectively. v0.1 shipped long ago — GoSD is on v0.6.5 — and this
milestone stayed open on hardware beans alone. Against its own definition of
done:

- **"`gosd build ./examples/hello --board=…` produces a flashable .img on a
  plain macOS/Linux machine with only Go installed (no root, no Docker)" —
  SHIPPED** (epic gosd-vi0n), and now for seven public boards rather than the
  two named here.
- **"Pi Zero 2W joins WiFi from credentials baked in at build time; Radxa
  Zero 3E gets a DHCP lease over Ethernet; the example app serves HTTP on :80
  and logs to serial" — SHIPPED and hardware-proven on both boards**
  (gosd-m9dj; gosd-nlzf session 1, where the Radxa took lease 192.168.1.233
  and answered on .local from macOS). The build-time credential baking this
  bullet describes was later replaced wholesale by the config tree (epic
  gosd-rw6n) — a supersession, not a gap.
- **"Boots the board into the example Go app in under 10 seconds from
  power-on" — NOT MET, and the target was retired rather than hit.**
  gosd-m9dj measured roughly 25s power-to-HTTP on the Pi Zero 2W and recorded
  the reason: the WiFi path pays association + DHCP + mDNS on top of boot,
  where wired boards land near 9s (rock-4se). The equivalent Radxa figure was
  never taken at all.

**What did not finish.** The Radxa Zero 3E's boot-time baseline, its 5x
power-cycle survival run, and its gadget / GbE / peripheral checks — six
unchecked items in gosd-nlzf, held up by serial: this board's 1.5Mbaud TX
garbles on the bench CP2102N, and full U-Boot visibility needs a CH340 cable.
That work moved with its epic gosd-v370 to the v0.7 milestone (gosd-dyoi).
COMPATIBILITY.md still shows the Radxa Zero 3E bring-up as "In progress" and
correctly continues to.

The hardware-kit bean gosd-s4t4 was completed alongside this one on evidence
rather than ticked boxes — its own summary sets out what is and is not
claimed.
