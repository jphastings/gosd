---
# gosd-lw7j
title: 'CLAUDE.md: Allwinner family guidance + activation fixture rule + stale fleet-tag wording'
status: completed
type: task
priority: normal
created_at: 2026-08-07T19:04:46Z
updated_at: 2026-08-07T19:05:33Z
---

Fold the cubie-a5e epic's durable lessons into CLAUDE.md (all proven 2026-08-06/07, epic gosd-h1wv):
- [x] Fix "Kernel pins are per-family": the mainline fleet tag now spans Rockchip + Allwinner (cubie-a5e) + qemu-virt, no longer "the Rockchip boards + qemu-virt"
- [x] New Allwinner-family bullet in Board work: sunxi boot chain (single u-boot-sunxi-with-spl.bin raw-written at 8KiB; BootROM also probes 128KiB), blob-free via TF-A BL31 from the commit-pinned jernejsk a523 fork until mainline gains sun55i_a523 (bean gosd-cjr6 tracks the repin), SCP unused on A523, USB gadget is MUSB not dwc3, per-board LPDDR4 DRAM tuning lives in the U-Boot defconfig (a new Allwinner board needs its own merged defconfig, not a sibling's), and the defconfig-rename trap: U-Boot board configs can be RENAMED between list posting and merge (radxa-a5e_defconfig → radxa-cubie-a5e_defconfig) — verify against the tree at the pinned tag, never the mailing-list name
- [x] Board activation rule: flipping a board public must add its artifacts to cmd/gosd/testdata/fake-artifacts/ (the tripwire integration tests build the default set), and a warm real-HOME artifact cache MASKS the tripwire locally — verify cmd/gosd tests with an isolated HOME before pushing an activation

## Summary of Changes

Reworded the per-family kernel-pin bullet (fleet tag now spans three families; added the subagent-background-build rule alongside the existing backgrounding advice), added the Allwinner family-facts bullet (sunxi single raw write, TF-A fork pin + gosd-cjr6, MUSB gadget, defconfig DRAM tuning + rename trap), and added the activation fixture rule with the cache-masking caveat (PR #205's lesson). Carried the retro's planning beans (gosd-cjr6, gosd-vo75, gosd-2194) into the repo.
