---
# gosd-ufeh
title: End-user flash guide (screenshots, zero terminal)
status: todo
type: task
created_at: 2026-07-02T21:07:10Z
updated_at: 2026-07-02T21:07:10Z
parent: gosd-b22t
blocked_by:
    - gosd-pctc
---

docs/flashing.md written for a minimally technical audience, plus a template Go developers can copy into their own project READMEs.

Content: install Raspberry Pi Imager → Choose OS → Use custom → pick the .img → gear/customization dialog: set hostname + WiFi → flash → insert + power → open http://<hostname>.local. Screenshots of each Imager step (current Imager UI). Radxa variant: same flow minus WiFi, plug in Ethernet. Troubleshooting section: wrong WiFi password symptoms, finding the device without mDNS (router admin page), re-editing gosd.toml.

- [ ] Guide with screenshots
- [ ] Copy-paste README snippet for tool users ("How to install <YourApp> on a Raspberry Pi")
- [ ] Reviewed by someone non-technical if possible; note feedback here

## Acceptance
A person who has never used a terminal can follow it end to end on a Mac or Windows machine.
