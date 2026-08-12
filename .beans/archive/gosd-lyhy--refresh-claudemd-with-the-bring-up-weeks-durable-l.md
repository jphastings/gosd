---
# gosd-lyhy
title: Refresh CLAUDE.md with the bring-up week's durable lessons
status: completed
type: task
created_at: 2026-07-27T06:24:31Z
updated_at: 2026-07-27T06:24:31Z
---

Codify the 2026-07-24..27 hardware-bring-up lessons into CLAUDE.md so future agents inherit them: add pi-3b to the board list; correct the 'all boards pin the same kernel tag' claim (two per-family pins — the false premise cost gosd-anyp's research a day); scope the 'no runtime overlays' rule to Rockchip (Pi firmware applies overlays natively; dwc2.dtbo ships via manifest per gosd-spjt); document the defconfig promotion trap (hwsim phantoms, legacy gadget zoo, RUNTIME_UARTS=0 — three hardware-found instances) and the Pi DTB lineage/compatible-binding rule (gosd-1ey5, gosd-spjt); the mdlayher/netlink Request-flag rule (gosd-anyp); activation sequencing incl. workflow_dispatch pre-merge testing (gosd-7wv9); beans-create positional-title gotcha; releases-are-cheap note.

## Summary of Changes

CLAUDE.md: corrected the same-fleet-tag claim to the per-family pin reality (Rockchip tag vs Pi downstream commit — the gosd-anyp false premise); scoped the no-runtime-overlays rule to Rockchip with the Pi dwc2.dtbo manifest pattern; added the Pi defconfig promotion-trap bullet (hwsim, legacy gadget zoo, RUNTIME_UARTS — gosd-6nl2/spjt/md4w) and the DTB-lineage/compatible-binding bullet (gosd-1ey5/spjt); documented activation sequencing incl. pre-merge workflow_dispatch testing (gosd-7wv9); noted releases are cheap; added the mdlayher/netlink Request-flag rule (gosd-anyp) to code conventions; recorded the beans-create positional-title gotcha. Board list/Target updates had already landed via PR #127.
