---
gosd: note
---

#### Updates are by reflashing, permanently — and the boot-time claim is now the measured one

Two product decisions, both of which change what the documentation promises.

**Over-the-network updates are dropped.** GoSD will not gain an OTA update
mechanism: reflashing the card is the update path, permanently. Plain
Raspberry Pi Imager reflash was already the documented baseline, and it keeps
what a device has — a `--data-size=expand` image re-adopts its own data
partition on first boot, and the config store puts the operator's hostname,
WiFi credentials and hand-edited settings back onto the newly flashed card. So
an upgrade costs one Imager run and loses neither data nor settings. The cost,
stated plainly: fixing a fielded device needs physical access to its card.
Consequence worth knowing if you audit what an image listens on — mDNS is now
the only network listener in `gosd-init`, with no sanctioned exception
pending. The design that was declined is kept in the repository as a record of
a road not taken.

**The boot-time claim was wrong in both directions and is now measured.** The
README promised "under 5 seconds, WiFi included". On real hardware your app is
running about 10 seconds after power-on, and a wired board answers on
`hostname.local` in about the same (ROCK 4SE: ~9.2s power-to-HTTP). Over WiFi,
expect ~25s — association, DHCP and mDNS announcement all happen *after* your
app is already serving. "The app is running" and "the app is reachable" are
different numbers, and the README now says so.

Also corrected: the feature list said USB gadget mode could present the board
as a USB Ethernet device. It cannot — CDC-ACM serial and mass storage are what
the `gadget` package builds today, as the compatibility matrix has always
said.
