---
# gosd-sc9w
title: 'v0.1 — Hello GoSD: both boards boot a Go app with network'
status: todo
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
