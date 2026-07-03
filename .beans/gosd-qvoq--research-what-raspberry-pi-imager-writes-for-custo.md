---
# gosd-qvoq
title: 'Research: what Raspberry Pi Imager writes for custom images (with captured fixtures)'
status: todo
type: task
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-02T21:07:10Z
parent: gosd-b22t
---

Settle exactly what provisioning data Raspberry Pi Imager leaves on the boot partition when a user flashes a CUSTOM image (Use custom) with OS customization filled in (WiFi SSID/password, hostname, user, locale). Everything downstream depends on this being empirical, not assumed.

Method:
- [ ] Install the current Raspberry Pi Imager release; note exact version
- [ ] Create a dummy .img with a FAT32 first partition (use our image writer), flash to a spare card/USB stick with customization enabled, then read back every file Imager added or modified. Repeat for: WiFi+hostname set; hostname only; everything set
- [ ] Read the rpi-imager source (github.com/raspberrypi/rpi-imager, OS customization code) to confirm which mechanism applies to custom images and when it chooses cloud-init (user-data/network-config), firstrun.sh + cmdline.txt edit, or custom.toml — and whether the WiFi PSK is written plaintext or PBKDF2-hashed
- [ ] Also check: does Imager behave differently if the image has no cmdline.txt (Radxa images)? Does it corrupt anything we need?
- [ ] Commit every captured file verbatim as `internal/provision/testdata/imager-<version>/<scenario>/...`
- [ ] Write docs/provisioning-formats.md: formats found, field-by-field extraction table (SSID, PSK+hash format, hostname, user), version differences, and a recommendation for parser precedence

## Acceptance
Fixtures committed for at least 3 scenarios from a real Imager run; doc answers plaintext-vs-hashed PSK and the format-selection question with source links.
