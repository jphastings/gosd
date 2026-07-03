---
# gosd-sc9w
title: 'v0.1 — Hello GoSD: both boards boot a Go app with network'
status: todo
type: milestone
created_at: 2026-07-02T20:46:56Z
updated_at: 2026-07-02T20:46:56Z
---

MVP milestone. Definition of done:

- `gosd build ./examples/hello --board=pi-zero-2w` and `--board=radxa-zero-3e` each produce a flashable .img on a plain macOS/Linux machine with only Go installed (no root, no Docker at build time).
- Flashing the image (dd or Raspberry Pi Imager, no customization) boots the board into the example Go app in under 10 seconds from power-on.
- Pi Zero 2W joins WiFi from credentials baked into the image at build time (end-user Imager provisioning is v0.2); Radxa Zero 3E gets a DHCP lease over Ethernet.
- Example app serves HTTP on :80 and logs to serial console.

Both boards are developed in parallel — neither is 'the port'.
