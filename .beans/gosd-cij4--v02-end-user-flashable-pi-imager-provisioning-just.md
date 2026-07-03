---
# gosd-cij4
title: 'v0.2 — End-user flashable: Pi Imager provisioning just works'
status: todo
type: milestone
created_at: 2026-07-02T20:47:11Z
updated_at: 2026-07-02T20:47:11Z
---

The 'mum can flash it' milestone. Definition of done:

- A Go developer publishes a .img built by GoSD. An end user opens Raspberry Pi Imager, picks 'Use custom' → the .img, enters WiFi SSID/password + hostname in Imager's OS-customization dialog, flashes, inserts card, powers on — and the device appears on the network. Zero terminal usage, no gok/gosd install for the end user.
- Device is discoverable as <hostname>.local via mDNS.
- Radxa Zero 3E path (Ethernet, no WiFi) works by just flashing with no customization; hostname editable via gosd.toml on the boot partition.
- Published artifacts (kernels, bootloaders) are versioned, checksummed, and downloaded/cached automatically by the CLI.
- Docs: quickstart for Go devs; flash guide with screenshots for end users.
