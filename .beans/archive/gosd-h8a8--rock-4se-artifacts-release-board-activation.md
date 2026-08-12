---
# gosd-h8a8
title: 'ROCK 4SE: artifacts release + board activation'
status: completed
type: task
priority: normal
created_at: 2026-07-13T13:18:13Z
updated_at: 2026-07-17T00:24:31Z
parent: gosd-cuym
blocked_by:
    - gosd-0vvh
---

The tag-first/bump-second dance per docs/artifacts.md:107-126. Precondition: A2's kernel job and A3's uboot job are green on main and package-and-release covers rock-4se (needs: list, download steps, provenance, files:). JP pushes artifacts/v0.5.0; then ONE activation PR.

## Activation PR contents

- Flip rock-4se from RegisterInternal to boards.Register in cmd/gosd/build.go
- Bump internal/artifacts.Version to v0.5.0
- internal/catalog/catalog.go: boardDisplayNames["rock-4se"] = "Radxa ROCK 4SE" (no Imager device tag — raw-ID fallback like radxa/nanopi); regenerate golden os_list.json
- COMPATIBILITY.md: new column + footnotes (code-complete-not-hardware-verified until A-bring-up; WiFi/BT out of scope; NVMe+exFAT+mass-storage in stock kernel)
- CLAUDE.md Board IDs line, README, docs/board-build-tags.md if not already done

## Todo

- [x] Verify package-and-release wiring covers both rock-4se jobs — done pre-tag via a workflow_dispatch run (29539839333, all jobs green): dist/rock-4se.tar.zst packaged with the same five files as radxa-zero-3e, manifest source carrying kernel+uboot+tfa provenance.
- [x] JP pushes artifacts/v0.5.0 tag (2026-07-17)
- [x] Activation PR (list above)
- [x] Three-way verification recorded here: clean-HOME real-network all-boards build; offline dead-proxy cache re-run; content spot-check (dtc shows header I2C okay; kernel.config carries EXFAT/NVME/MASS_STORAGE =y)

## Summary of Changes

Branch `bean/gosd-h8a8-rock4se-activation`.

- cmd/gosd/build.go: `RegisterInternal` → `Register` for rock-4se.
- internal/artifacts.Version: v0.4.0 → v0.5.0.
- internal/catalog: `boardDisplayNames["rock-4se"] = "Radxa ROCK 4SE"` (no
  Imager device tag — raw-ID fallback); fakeBuild + golden os_list.json
  regenerated for five boards.
- cmd/gosd/build_integration_test.go: default all-boards test now expects
  five images (qemu-virt back to being the only internal board); new
  `TestBuildCatalogForRock4SEWritesEntry` mirroring the nanopi activation
  test; stale internal-only comments updated.
- COMPATIBILITY.md: ROCK 4SE column + six new footnotes (blob-free
  U-Boot/TF-A provenance; WiFi/BT hardware present but out of scope; eMMC
  is an optional module with the Arasan driver present only incidentally;
  the dwc3_0 OTG-port best-guess; a new NVMe+exFAT row — the board's
  raison d'être); count-bearing footnotes de-counted per the docs
  convention.
- CLAUDE.md Board IDs + arch lists; README board list;
  docs/board-build-tags.md internal-only note removed.

## Release + verification record (2026-07-17)

**Release:** the artifacts/v0.5.0 tag run's nine build jobs all succeeded
first try; the Publish step failed twice on a GitHub platform incident
("Degraded REST API Availability", major, confirmed on githubstatus.com —
the API returned HTML error pages to action-gh-release). Re-running the
failed job after the incident resolved published the release with all
seven assets. No workflow changes were needed.

**Three-way verification, all against the real published release, from
this branch:**

1. **Clean-machine build**: fresh empty `HOME`, no `--board`/
   `--artifacts-dir` → real download of v0.5.0 → all FIVE public images
   built (`hello-{pi-zero-2w,pi-zero-w,radxa-zero-3e,nanopi-zero2,rock-4se}.img`).
2. **Offline re-run**: same HOME, `HTTP(S)_PROXY` pointed at a dead port →
   all five images rebuilt entirely from the artifact cache.
3. **Content spot-check** on the downloaded rock-4se artifacts:
   - `dtc -I dtb` on `rk3399-rock-4se.dtb`: i2c2/i2c6/i2c7 (ff120000/
     ff150000/ff160000) all `status = "okay"`; spi1 (ff1d0000) `okay` with
     the `rohm,dh2228fv` spidev child; dwc3@fe800000
     `dr_mode = "peripheral"`.
   - `kernel.config`: `CONFIG_EXFAT_FS`, `CONFIG_BLK_DEV_NVME`,
     `CONFIG_PCIE_ROCKCHIP_HOST`, `CONFIG_USB_CONFIGFS_MASS_STORAGE`,
     `CONFIG_PHY_ROCKCHIP_TYPEC` all `=y`.
