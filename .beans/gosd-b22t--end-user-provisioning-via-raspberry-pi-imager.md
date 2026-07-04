---
# gosd-b22t
title: End-user provisioning via Raspberry Pi Imager
status: todo
type: epic
priority: normal
created_at: 2026-07-02T20:50:25Z
updated_at: 2026-07-04T13:09:02Z
parent: gosd-cij4
---

Make Raspberry Pi Imager's OS-customization dialog (WiFi SSID/password, hostname, locale) work against GoSD images. Imager writes provisioning files onto the first FAT partition of a custom image; gosd-init reads them at boot instead of running the shell scripts they were designed for.

Key risk to burn down first: exactly WHAT Imager writes for 'Use custom' images across current Imager versions (firstrun.sh + cmdline.txt edit, and/or cloud-init user-data/network-config, and/or custom.toml) — the research task must settle this with real captured samples before the parser is built.

Also includes the non-Imager fallback (hand-editable gosd.toml on GOSD-BOOT) and mDNS discoverability, since 'find your headless device' is the other half of the end-user problem.

## Strategic finding from gosd-qvoq source analysis (2026-07-04)
Raspberry Pi Imager GUI DISABLES the OS-customization dialog for "Use custom" local .img files — customization is gated on catalog metadata (init_format) that local files never carry. The imagined end-user flow (flash local .img + enter WiFi in dialog) does not work as-is. Candidate paths, pending JP decision: (a) developers publish an os_list.json catalog entry (with init_format) hosted alongside their image, users add the repo URL to Imager; (b) rely on rpi-imager-cli flags; (c) lean on gosd.toml hand-editing as the primary flow; (d) ship our own minimal flasher later. See docs/provisioning-formats.md (PR #18) for citations. The provisioning parser (gosd-pctc) remains worthwhile regardless: any of (a)/(b) still writes the standard files.
