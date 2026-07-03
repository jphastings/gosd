---
# gosd-30jz
title: USB Ethernet gadget mode (ECM + RNDIS) with built-in DHCP server
status: todo
type: task
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-02T21:10:00Z
parent: gosd-jge2
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
