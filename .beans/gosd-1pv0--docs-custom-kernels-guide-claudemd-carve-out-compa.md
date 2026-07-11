---
# gosd-1pv0
title: 'Docs: custom-kernels guide, CLAUDE.md carve-out, COMPATIBILITY row'
status: todo
type: task
created_at: 2026-07-11T07:41:32Z
updated_at: 2026-07-11T07:41:32Z
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

- [ ] `docs/custom-kernels.md` incl. worked DVB example
- [ ] CLAUDE.md carve-out + source-of-truth updates
- [ ] COMPATIBILITY.md row
- [ ] README pointer
