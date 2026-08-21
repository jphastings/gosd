---
# gosd-30jz
title: USB Ethernet gadget mode (ECM + RNDIS) with built-in DHCP server
status: todo
type: task
priority: normal
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-08-21T04:42:20Z
parent: gosd-q6g6
blocked_by:
    - gosd-uo9f
---

Device-as-network-interface: plug the board into any computer via USB and reach the app at a fixed address — no WiFi/Ethernet needed at all. This is the best minimally-technical-user story for the Pi Zero 2W.

- [ ] Add ECM (macOS/Linux) and RNDIS (Windows) functions to the gadget package; composite gadget with both via config c.1/c.2 (os_desc for Windows RNDIS matching — research the exact configfs incantation, document in code)
- [ ] gosd-init: when gadget-ethernet is enabled, configure usb0 with 10.55.0.1/24 and run a minimal DHCPv4 server (github.com/insomniacslk/dhcp server4) offering 10.55.0.2 to the host
- [ ] mDNS answers on usb0 too (hostname.local works over the USB cable)
- [ ] Builder flag --usb-ethernet; document host-side expectations per OS in docs/usb-gadget.md
- [ ] Hardware test: macOS, Windows 11, Linux hosts against the Pi Zero 2W; record per-OS results here (Radxa too, lower priority)

## Acceptance
Pi Zero 2W plugged into a Mac via USB alone: http://hostname.local loads within 15s with zero configuration.


## Deliberately kept deferred (JP, 2026-08-21)

Reviewed on 2026-08-21 in the pass that closed the OTA chain and the audio
extras, and **deliberately left deferred rather than scrapped or scheduled**.
This note exists so that a later reader does not mistake its age for neglect:
it was looked at, and the answer was "not yet", not "forgotten".

It is the largest thing v0.3 promised and did not deliver. "Plug the board
into any computer over one USB cable and reach the app at a fixed address" is
still the best story GoSD has for a minimally-technical user on a Pi Zero 2W —
no WiFi credentials, no network, no terminal — and nothing has replaced it.
The prerequisite (`gadget/`, the pure-Go configfs gadget library, bean
gosd-uo9f) is shipped and proven with CDC-ACM serial and mass storage, so the
missing pieces are the ECM and RNDIS functions, the composite gadget with the
Windows `os_desc` incantation, a minimal DHCPv4 server on `usb0`, mDNS on
`usb0`, and the three-OS hardware pass.

Still deferred, not closed: it stays on the list exactly as written above.
