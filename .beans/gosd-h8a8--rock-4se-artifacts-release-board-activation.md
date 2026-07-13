---
# gosd-h8a8
title: 'ROCK 4SE: artifacts release + board activation'
status: todo
type: task
priority: normal
created_at: 2026-07-13T13:18:13Z
updated_at: 2026-07-13T13:26:10Z
parent: gosd-cuym
blocked_by:
    - gosd-0vvh
---

The tag-first/bump-second dance per docs/artifacts.md:107-126. Precondition: A2's kernel job and A3's uboot job are green on main and `package-and-release` covers rock-4se (needs: list, download steps, provenance, files:). JP pushes `artifacts/v0.5.0`; then ONE activation PR.

## Activation PR contents

- Flip rock-4se from RegisterInternal to `boards.Register` in cmd/gosd/build.go
- Bump `internal/artifacts.Version` to v0.5.0
- `internal/catalog/catalog.go`: boardDisplayNames["rock-4se"] = "Radxa ROCK 4SE" (no Imager device tag — raw-ID fallback like radxa/nanopi); regenerate golden_os_list.json
- COMPATIBILITY.md: new column + footnotes (code-complete-not-hardware-verified until A-bring-up; WiFi/BT out of scope; NVMe+exFAT+mass-storage in stock kernel)
- CLAUDE.md Board IDs line, README, docs/board-build-tags.md if not already done

## Todo

- [ ] Verify package-and-release wiring covers both rock-4se jobs
- [ ] JP pushes artifacts/v0.5.0 tag
- [ ] Activation PR (list above)
- [ ] Three-way verification recorded here: clean-HOME real-network all-boards build; offline dead-proxy cache re-run; content spot-check (dtc shows header I2C okay; kernel.config carries EXFAT/NVME/MASS_STORAGE =y)
