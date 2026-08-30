---
# gosd-l0xq
title: 'Pi CM4: verify SD-card boot via SDWire once USB is unwedged'
status: todo
type: task
created_at: 2026-08-30T13:29:49Z
updated_at: 2026-08-30T13:29:49Z
parent: gosd-7676
---

## What

The board profile's actual design is SD-boot only (gosd-1tk8); the
eMMC-boot path used in gosd-5trv's session (2026-08-30) is an unscoped
workaround, not a substitute for verifying the real path.

Once the SDWire's USB connection un-wedges (JP: "The SDWire can get
wedged; let's leave it for now" — it stopped enumerating on the Mac
mid-session, power-cycling the meross-controlled outlet didn't help,
suspected USB-side wedge rather than power), flash the built pi-cm4
image to the CM4's SD card the normal way and confirm the common
bring-up core: boot → serial console → network up (DHCP) → mDNS + HTTP
reachable → power-cycle survival of /data.

Not urgent — no reason to believe the SD path behaves differently from
the eMMC path content-wise (same FAT-based GPU-ROM boot chain either
way), but it's the documented/supported path and deserves its own real
confirmation.
