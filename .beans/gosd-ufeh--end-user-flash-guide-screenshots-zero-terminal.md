---
# gosd-ufeh
title: End-user flash guide (screenshots, zero terminal)
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-06T13:18:06Z
parent: gosd-b22t
blocked_by:
    - gosd-pctc
---

docs/flashing.md written for a minimally technical audience, plus a template Go developers can copy into their own project READMEs.

Content: install Raspberry Pi Imager → Choose OS → Use custom → pick the .img → gear/customization dialog: set hostname + WiFi → flash → insert + power → open http://<hostname>.local. Screenshots of each Imager step (current Imager UI). Radxa variant: same flow minus WiFi, plug in Ethernet. Troubleshooting section: wrong WiFi password symptoms, finding the device without mDNS (router admin page), re-editing gosd.toml.

- [x] Guide with screenshots
- [x] Copy-paste README snippet for tool users ("How to install <YourApp> on a Raspberry Pi")
- [ ] Reviewed by someone non-technical if possible; note feedback here

## Acceptance
A person who has never used a terminal can follow it end to end on a Mac or Windows machine.


## Summary of Changes

Wrote docs/flashing.md: a second-person, jargon-free walkthrough for a
minimally technical end user, using seven real Raspberry Pi Imager v2.0.10
screenshots (JP's bench session, 2026-07-06) copied into
docs/images/flashing/ — one embedded image per numbered step, each with
descriptive alt text, mirroring the real UI 1:1 (App options → Content
Repository → custom URL → choose device → choose app → hostname → WiFi).
Covers install, writing the card, first power-on (no screen needed, allow a
minute or two), and finding the device at http://<hostname>.local. Includes
a gentle troubleshooting section (app not in the list / No filtering for
non-Pi boards, .local not resolving, WiFi-typo recovery via the gosd.toml
fallback on the GOSD-BOOT drive) and a copy-paste README snippet section for
app developers, satisfying the second todo. Cross-linked from README.md's
quickstart step 4 and from docs/publishing.md.

Privacy-checked every screenshot before committing: all WiFi/hostname
values shown are Imager's own placeholders ("your-network", "your-device")
or example.com URLs, not real credentials.

Left unchecked: no non-technical reviewer was available in this session, so
that todo and the bean's on-hardware-flavored acceptance criterion (a real
first-time user following the guide end-to-end) remain open — bean stays
in-progress.
