---
# gosd-1pv0
title: 'Docs: custom-kernels guide, CLAUDE.md carve-out, COMPATIBILITY row'
status: completed
type: task
priority: normal
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T19:12:07Z
parent: gosd-47rm
blocked_by:
    - gosd-abya
    - gosd-07fl
---

Part of [[gosd-47rm]]. Documentation + convention updates once the command
exists ([[gosd-abya]]) and CI dogfoods it ([[gosd-07fl]]).

## Locked decisions

- New `docs/custom-kernels.md`: the two-tier story (stock artifacts vs
  `build-kernel`), a worked example (enable a USB DVB-T driver on
  pi-zero-2w: fragment with `CONFIG_MEDIA_SUPPORT=y`, `CONFIG_DVB_CORE=y`,
  `CONFIG_MEDIA_USB_SUPPORT=y`, `CONFIG_DVB_USB_V2=y` + chipset driver, plus
  the firmware blob entry), `gosd-kernel.toml` reference, Docker/Podman
  prerequisite + supported hosts, reproducibility notes, and the GPL
  provenance story (`source.json`; developers distributing images built from
  GPL kernels carry the same obligations we handle for stock artifacts).
  - The worked example must note the known pi-zero-2w wrinkle: enabling
    `CONFIG_MEDIA_SUPPORT` on the rpi tree can hit the `rp1-cfe` duplicate-
    symbol link failure (why our fragment disables it) — document the
    workaround discovered during implementation.
- `docs/artifacts.md`: release path now goes through `build-kernel` (may land
  with [[gosd-07fl]] instead; don't duplicate).
- **CLAUDE.md** locked-decisions updates:
  - build-purity carve-out: "`gosd build` never requires Docker;
    `gosd build-kernel` requires Docker or Podman, by design"
  - kernel-build source of truth is the Go `KernelSpec`, not shell scripts
  - amend the [[gosd-y0x3]] framing: developers never *have* to compile a
    kernel; `build-kernel` is the opt-in path
- **COMPATIBILITY.md**: add a "Custom kernel (`gosd build-kernel`)" row
  (per-board ✅/❌ with the usual code-complete caveat) in the same PR as
  whichever bean makes it true per board — this bean sweeps up whatever is
  left.
- README gets a one-paragraph pointer to `docs/custom-kernels.md` — no more.

## Todos

- [x] `docs/custom-kernels.md` incl. worked DVB example
- [x] CLAUDE.md carve-out + source-of-truth updates
- [x] COMPATIBILITY.md row
- [x] README pointer


## Summary of Changes

- New `docs/custom-kernels.md`: the two-tier story (stock artifacts by
  default, zero Docker; `gosd build-kernel` opt-in, Docker/Podman required),
  a quickstart, the full `gosd-kernel.toml` v1 reference (read directly from
  `internal/kernelconfig`/`internal/kernelspec`), overlay/caching/GPL
  provenance notes, and supported hosts.
- The worked example is the flagship USB DVB-T-on-pi-zero-2w case, and it is
  **proven, not aspirational**: built a throwaway `gosd` binary from `main`
  plus PR #70's not-yet-merged state-dir fix (kept out of this branch), ran
  a real `gosd build-kernel --board=pi-zero-2w` against local Docker with a
  `gosd-kernel.toml` + fragment enabling `CONFIG_MEDIA_SUPPORT` and the
  RTL28xxU DVB stack. It built successfully on the **first attempt** — no
  fragment iteration needed. Root-caused the documented rp1-cfe wrinkle by
  inspecting the pinned raspberrypi/linux tree directly (sparse-checkout of
  `drivers/media`): `bcm2711_defconfig` sets `CONFIG_EXPERT=y`, which
  defaults `CONFIG_MEDIA_PLATFORM_SUPPORT` to `y` the moment
  `CONFIG_MEDIA_SUPPORT` is on, which in turn default-enables both
  raspberrypi/linux's CSI camera front-end drivers (`rp1-cfe` and its
  in-tree replacement `rp1_cfe`) — with `CONFIG_MODULES` off, `make
  olddefconfig` promotes both from the defconfig's `=m` to a built-in `=y`,
  and they collide at link time on `dphy_start`/`dphy_stop`/`dphy_probe`.
  The fix, documented in the guide: explicitly `# CONFIG_VIDEO_RP1_CFE is
  not set` / `# CONFIG_VIDEO_RP1_CFE_DOWNSTREAM is not set` in the developer
  fragment. Verified by grepping the produced `kernel.config`: every DVB
  symbol is `=y`, both CFE symbols are absent, `CONFIG_MODULES` stays unset.
  Experiment artifacts (fragment, toml, log) are not committed — only quoted
  in the doc.
- `CLAUDE.md`: build-purity carve-out (`gosd build` never needs Docker,
  `gosd build-kernel` always does), a new "kernel-build source of truth is
  `internal/kernelspec`, not shell scripts" bullet, and an amendment to the
  third-party-blobs bullet noting developers never *have to* compile a
  kernel themselves.
- `COMPATIBILITY.md`: new "Custom kernel (`gosd build-kernel`)" row, ✅ for
  all four public boards, with a footnote recording the qemu-virt real-
  Docker-to-boot verification, the fake-artifact/CI-tested per-board status,
  and the proven (not just fake-tested) DVB worked example — plus the usual
  no-physical-hardware-yet caveat.
- `README.md`: one paragraph pointing at `docs/custom-kernels.md`.
