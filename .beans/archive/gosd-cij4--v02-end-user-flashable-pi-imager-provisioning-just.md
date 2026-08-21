---
# gosd-cij4
title: 'v0.2 — End-user flashable: Pi Imager provisioning just works'
status: completed
type: milestone
priority: normal
created_at: 2026-07-02T20:47:11Z
updated_at: 2026-08-21T01:42:38Z
---

The 'mum can flash it' milestone. Definition of done:

- A Go developer publishes a .img built by GoSD. An end user opens Raspberry Pi Imager, picks 'Use custom' → the .img, enters WiFi SSID/password + hostname in Imager's OS-customization dialog, flashes, inserts card, powers on — and the device appears on the network. Zero terminal usage, no gok/gosd install for the end user.
- Device is discoverable as <hostname>.local via mDNS.
- Radxa Zero 3E path (Ethernet, no WiFi) works by just flashing with no customization; hostname editable via gosd.toml on the boot partition.
- Published artifacts (kernels, bootloaders) are versioned, checksummed, and downloaded/cached automatically by the CLI.
- Docs: quickstart for Go devs; flash guide with screenshots for end users.

## Summary of Changes

All three children shipped — gosd-b22t (end-user provisioning), gosd-c54j
(qemu-virt) and gosd-y0x3 (artifact pipeline, CI and docs) — and the
definition of done above is met, with one amendment recorded when the
research landed.

**The amendment.** The first bullet describes flashing a local `.img` via
Imager's "Use custom" picker and filling in the OS-customization dialog.
gosd-qvoq proved that dialog is unreachable that way for ANY image, GoSD's
included: Imager gates it on catalog metadata a local file cannot carry. JP
chose the custom-repository catalog entry instead (2026-07-05, in CLAUDE.md),
which delivers the same end-user experience — the developer hosts an
`os_list.json` beside the image, the user pastes one URL into Imager's
settings and gets the full WiFi/hostname wizard, still with zero terminal use
and no gosd install. The screenshot-driven walkthrough in the flashing guide
is that flow end to end.

The remaining bullets hold as written: the device answers `<hostname>.local`
over mDNS; the Radxa Zero 3E boots from a plain flash with no customization
and takes a DHCP lease over Ethernet (proven on the bench, gosd-nlzf's first
session, where it also resolved and answered on `.local`); board artifacts
are versioned, checksummed and downloaded/cached by the CLI automatically;
and both docs audiences are served — the README quickstart plus the runtime
contract for Go developers, the screenshot flash guide for end users.

Note on ordering: the v0.1 milestone gosd-sc9w is still open, so these
numbers are not sequential. It is held by hardware beans only — the bring-up
kit (gosd-s4t4) and the Radxa Zero 3E boot-time/power-cycle baseline
(gosd-nlzf) — not by anything this milestone needed.
