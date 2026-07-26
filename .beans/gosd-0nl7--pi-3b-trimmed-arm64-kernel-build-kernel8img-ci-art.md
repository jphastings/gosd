---
# gosd-0nl7
title: 'Pi 3B: trimmed arm64 kernel build (kernel8.img) + CI artifacts job'
status: in-progress
type: task
priority: normal
created_at: 2026-07-25T23:22:20Z
updated_at: 2026-07-26T10:22:52Z
parent: gosd-xhc3
blocked_by:
    - gosd-ypg1
---

Second PR of epic gosd-xhc3: prove the pi-3b kernelspec entry with a real
`gosd build-kernel --board pi-3b` Docker build, and wire the board into the
artifact pipeline. Blocked by gosd-ypg1 (build-kernel resolves the board via
the registry — CLAUDE.md's registration-before-kernel coupling).

## Scope

- Run `gosd build-kernel --board pi-3b` locally (Docker/colima, backgrounded
  + poll the log per CLAUDE.md; bind mounts staged under $HOME, beans
  gosd-0p21/gosd-l4y9). Outputs: kernel8.img + bcm2710-rpi-3-b.dtb.
- Commit the generated `build/boards/pi-3b/kernel.config` with the provenance
  header (records the last actual build; never hand-edited — gosd-z9l4
  precedent).
- Verify every fragment =y survived olddefconfig (the build asserts
  RequiredY itself), and specifically spot-check in the recorded config:
  `CONFIG_USB_DWCOTG=y`, `CONFIG_USB_NET_SMSC95XX=y`,
  `CONFIG_SERIAL_8250_RUNTIME_UARTS=1`, and that the gadget/dwc2 block is
  gone or inert.
- `.github/workflows/build-artifacts.yml`: add the `pi-3b-kernel` job plus
  packaging/provenance/release entries (pi-zero-w's gosd-s7fk pattern);
  verify wiring with a `gh workflow run build-artifacts.yml --ref <branch>`
  dispatch and record the run URL here (gosd-h8a8's pre-tag verification
  pattern).

## Locked decisions

- Kernel source: raspberrypi/linux at piZeroCommitRef (the Pi fleet pin) —
  no new pin, no single-board bump.
- Do NOT touch internal/artifacts.Version (tag-first/bump-second; the bump
  is gosd-7wv9's).
- No COMPATIBILITY.md changes (board still internal-only).

## Todo

- [x] Local gosd build-kernel run green; kernel.config committed with provenance
- [x] Fragment-survival spot-checks recorded here
- [ ] build-artifacts.yml job + packaging entries; dispatch run URL recorded
- [x] Quality gates + actionlint on the workflow file


## Build provenance & fragment-survival checks (2026-07-26)

- The committed `build/boards/pi-3b/kernel.config` records the real local
  `gosd build-kernel --board pi-3b` Docker (colima) build run on this
  machine 2026-07-26 — the same build whose `--artifacts-dir` output fed the
  maiden hardware boot (see gosd-xhc3/gosd-f5xm). That run predates
  gosd-oq0z's fragment change, but the only functional delta gosd-oq0z made
  was asserting `CONFIG_USB_LAN78XX=y` — which this build's config already
  resolves `=y` from the bcm2711_defconfig baseline (the exact 'defconfig
  luck' gosd-oq0z existed to eliminate), so the recorded config is
  byte-identical to what the current fragment resolves to. Per CLAUDE.md the
  post-oq0z re-run (a full ~75 min rebuild — gosd-oq0z deliberately re-keys
  the cache for the second DTB) was not repeated just to re-emit an identical
  config; CI's dispatch run below builds with the current fragment + both
  DTBs end-to-end.
- All 43 explicit `CONFIG_*=` lines in the current kernel.fragment appear
  verbatim in the recorded config (checked mechanically), including the
  bean's named spot-checks: `CONFIG_USB_DWCOTG=y`,
  `CONFIG_USB_NET_SMSC95XX=y`, `CONFIG_SERIAL_8250_RUNTIME_UARTS=1`, plus
  gosd-oq0z's `CONFIG_USB_LAN78XX=y`.
- Gadget/dwc2 block is gone: `# CONFIG_USB_DWC2 is not set` and
  `# CONFIG_USB_GADGET is not set` in the recorded config.
- Of the fragment's 47 `is not set` lines, only `CONFIG_DEBUG_KERNEL=y`
  survives re-enabled by olddefconfig — identical to pi-zero-2w's and
  pi-zero-w's committed configs (noted in gosd-s7fk), so not a pi-3b
  regression.

## Release-window note

artifacts/v0.7.0 (tagged 2026-07-26 at a752d3f, the gosd-oq0z merge) shipped
WITHOUT pi-3b: the workflow at that tag had no pi-3b-kernel job, so the
release carries the Pi-Zero fixes only. This bean adds the job + packaging,
so the NEXT artifacts release is the first that can carry pi-3b — per the
epic's batching decision it rides whichever release window comes first
(gosd-36yy's fleet bump or an earlier forced release), then gosd-7wv9
activates the board against it.
