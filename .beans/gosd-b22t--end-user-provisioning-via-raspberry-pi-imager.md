---
# gosd-b22t
title: End-user provisioning via Raspberry Pi Imager
status: todo
type: epic
created_at: 2026-07-02T20:50:25Z
updated_at: 2026-07-02T20:50:25Z
parent: gosd-cij4
---

Make Raspberry Pi Imager's OS-customization dialog (WiFi SSID/password, hostname, locale) work against GoSD images. Imager writes provisioning files onto the first FAT partition of a custom image; gosd-init reads them at boot instead of running the shell scripts they were designed for.

Key risk to burn down first: exactly WHAT Imager writes for 'Use custom' images across current Imager versions (firstrun.sh + cmdline.txt edit, and/or cloud-init user-data/network-config, and/or custom.toml) — the research task must settle this with real captured samples before the parser is built.

Also includes the non-Imager fallback (hand-editable gosd.toml on GOSD-BOOT) and mDNS discoverability, since 'find your headless device' is the other half of the end-user problem.
