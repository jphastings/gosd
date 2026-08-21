---
# gosd-b22t
title: End-user provisioning via Raspberry Pi Imager
status: completed
type: epic
priority: normal
created_at: 2026-07-02T20:50:25Z
updated_at: 2026-08-21T01:42:20Z
parent: gosd-cij4
---

Make Raspberry Pi Imager's OS-customization dialog (WiFi SSID/password, hostname, locale) work against GoSD images. Imager writes provisioning files onto the first FAT partition of a custom image; gosd-init reads them at boot instead of running the shell scripts they were designed for.

Key risk to burn down first: exactly WHAT Imager writes for 'Use custom' images across current Imager versions (firstrun.sh + cmdline.txt edit, and/or cloud-init user-data/network-config, and/or custom.toml) — the research task must settle this with real captured samples before the parser is built.

Also includes the non-Imager fallback (hand-editable gosd.toml on GOSD-BOOT) and mDNS discoverability, since 'find your headless device' is the other half of the end-user problem.

## Strategic finding from gosd-qvoq source analysis (2026-07-04)
Raspberry Pi Imager GUI DISABLES the OS-customization dialog for "Use custom" local .img files — customization is gated on catalog metadata (init_format) that local files never carry. The imagined end-user flow (flash local .img + enter WiFi in dialog) does not work as-is. Candidate paths, pending JP decision: (a) developers publish an os_list.json catalog entry (with init_format) hosted alongside their image, users add the repo URL to Imager; (b) rely on rpi-imager-cli flags; (c) lean on gosd.toml hand-editing as the primary flow; (d) ship our own minimal flasher later. See docs/provisioning-formats.md (PR #18) for citations. The provisioning parser (gosd-pctc) remains worthwhile regardless: any of (a)/(b) still writes the standard files.

## Decision (2026-07-05, JP)
Flashing path chosen: Option A (Imager custom-repository catalog entry, init_format=cloudinit) as the flagship flow, Option C (gosd.toml hand-editing, already shipped) as the universal fallback. firstrun.sh parsing is out of scope. Recorded in CLAUDE.md.

## Summary of Changes

An end user with no terminal can put a developer's app on a card and find it
on the network. gosd-qvoq's research settled the key risk with captured
Imager fixtures — and produced the finding that reshaped the epic: Imager's
GUI disables OS customization for "Use custom" local files, because the
wizard is gated on catalog metadata a local file never carries. JP's
2026-07-05 decision (recorded in CLAUDE.md) picked the custom-repository
catalog as the flagship flow instead: gosd-t6cs made `gosd build --catalog
--publish-base-url` emit an `os_list.json` entry declaring
`init_format: "cloudinit"`, the developer hosts it beside the image, and
pasting that URL into Imager gives flashers the full WiFi/hostname wizard.
gosd-pctc built gosd-init's provisioning parser for the cloud-init files
Imager writes (`firstrun.sh` stays out of scope, log-and-point). gosd-tds2
shipped the hand-editable fallback that works with any flasher, and gosd-r796
made the device answer `<hostname>.local`. gosd-ufeh wrote the
screenshot-driven, jargon-free flash guide a developer can send to
non-technical people directly.

Two things have moved since, without reopening scope: epic gosd-rw6n replaced
the single hand-editable boot file with the per-attribute config tree, and
made the cloud-init seed something gosd-init CONSUMES — read, durably
deleted, then written into the tree — so a wizard's answers become ordinary
settings rather than a competing source of truth. Everything this epic built
survives inside that model.
