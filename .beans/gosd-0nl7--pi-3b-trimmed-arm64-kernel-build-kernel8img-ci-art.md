---
# gosd-0nl7
title: 'Pi 3B: trimmed arm64 kernel build (kernel8.img) + CI artifacts job'
status: todo
type: task
priority: normal
created_at: 2026-07-25T23:22:20Z
updated_at: 2026-07-25T23:22:42Z
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

- [ ] Local gosd build-kernel run green; kernel.config committed with provenance
- [ ] Fragment-survival spot-checks recorded here
- [ ] build-artifacts.yml job + packaging entries; dispatch run URL recorded
- [ ] Quality gates + actionlint on the workflow file
