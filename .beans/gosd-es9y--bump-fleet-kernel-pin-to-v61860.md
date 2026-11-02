---
# gosd-es9y
title: Bump fleet kernel pin to v6.18.60
status: in-progress
type: task
priority: normal
created_at: 2026-11-02T02:00:28Z
updated_at: 2026-11-02T02:00:28Z
---

Bump `fleetKernelTag` from v6.18.37 to v6.18.60 (newest numbered 6.18.x on kernel.org stable as of 2026-11-02; no -rc).

## Why
The mainline fleet (radxa-zero-3e, rock-4se, turing-rk1, nanopi-zero2, cubie-a5e, jetson-nano, qemu-virt) sits 23 LTS point releases behind on a line that is still maintained. This is maintenance, not capability: no new driver or board is unlocked. Kernel pins are per-family and bumped family-wide, so all seven move together. The Pi boards use a separate downstream commit pin and are untouched.

## Release order
Tag-first, bump-second: this PR ships the pin plus an `artifacts:` change file only. `internal/artifacts.Version` / `ManifestSHA256` are NOT touched; a follow-up PR bumps them after the artifacts release is published.

## Todo
- [x] Change `fleetKernelTag`
- [x] Sweep remaining `6.18.37` mentions and classify
- [x] Prove every fleet DTS patch applies at v6.18.60
- [x] Change file (`artifacts: patch`)
- [ ] Pre-merge artifacts build via `workflow_dispatch` goes green
- [ ] Follow-up PR bumping `internal/artifacts.Version` after release

## Patch application at v6.18.60 (f45ed76d6791)
Sparse tree of arch/arm64/boot/dts/{rockchip,allwinner}; fresh tree per board, `patch -p1 --forward --dry-run` then for real, in order. No fuzz, offset or reject on any patch.
- radxa-zero-3e: 0001 i2c3, 0002 spi3, 0003 led-retain-state clean
- rock-4se: 0001 i2c, 0002 spi, 0003 usb-dwc3-peripheral, 0004 led-retain-state clean
- nanopi-zero2: 0001 i2c5, 0002 spi1, 0003 led-retain-state clean
- cubie-a5e: 0001 gadget-variant-dtb, 0002 led-retain-state clean
- turing-rk1, jetson-nano, qemu-virt: no DTS patches

Not exercised: the examples/chime and examples/sattrack kernel recipes' own patches (user recipes, not fleet specs).

## Sweep of 6.18.37 mentions
Updated: `internal/kernelspec/kernelspec.go` (`fleetKernelTag`, the only mention of the current pin).

Left, all historical or verification records:
- `internal/artifacts/artifacts.go` doc comment: describes what that release shipped.
- `internal/boards/nanopizero2/templates/templates.go`: console alias verified at that tag.
- `build/boards/{rock-4se,cubie-a5e,jetson-nano}/kernel/kernel-fragment.config` header comments: symbols verified against that tree; not re-verified.
- `build/boards/*/kernel.config` snapshots: rewritten only by a real `gosd build-kernel`, never hand-edited.
- `examples/chime/kernel/*.fragment`: reproduction commands for the defconfig the denied symbols were read from.
- `docs/development/*.md` (turing-rk1, radxa-zero-3e, nanopi-zero2): bench/verification history.
- `.beans/**` (incl. archive): recorded history of other beans. `gosd-woox` untouched (open PR #405).
