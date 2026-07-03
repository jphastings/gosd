---
# gosd-v2w1
title: 'Spike: over-the-network app updates (A/B scheme) — design doc only'
status: todo
type: task
priority: low
created_at: 2026-07-02T21:10:00Z
updated_at: 2026-07-02T21:10:00Z
parent: gosd-jge2
---

Design, do not build. docs/design/ab-updates.md answering:

- [ ] What gokrazy does (A/B root partitions, HTTP push, boot-success watchdog) and what transfers to the GoSD initramfs-only architecture (likely: two kernel+initramfs pairs on GOSD-BOOT, a boot-slot flag file for Pi config.txt vs extlinux fallback semantics on U-Boot — investigate `sysboot`/bootcount)
- [ ] Failure model: power loss mid-update, bad app that boots but crashes (watchdog + rollback), FAT corruption
- [ ] How the developer pushes: `gosd push <host>` against a gosd-init update endpoint — authn story (per-image key baked at build?)
- [ ] Recommendation + task breakdown for v0.4

## Acceptance
Doc reviewed; follow-up beans created for the chosen design.
