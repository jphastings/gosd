---
# gosd-ci98
title: Assert the LED kernel symbols in every board fragment
status: todo
type: task
priority: normal
created_at: 2026-08-17T20:10:44Z
updated_at: 2026-08-17T20:25:04Z
---

The boot-state LED (gosd-xtcs) depends on kernel symbols that no GoSD kernel
fragment currently asserts. `LEDS_CLASS`, `LEDS_GPIO`, `LEDS_TRIGGER_TIMER`
and (for pi-3b) `GPIO_BCM_VIRT` are all `=y` in every board's released
kernel.config today, but only because the upstream defconfigs happen to set
them. A kernel bump could silently drop any of them and take a user-facing
feature with it, with nothing in the tree recording that we depend on them.

Fragments are the assertion; `build/boards/*/kernel.config` is a stale
snapshot and is not.

## Locked decisions

- Add the symbols to each board's fragment: `NEW_LEDS`, `LEDS_CLASS`,
  `LEDS_GPIO`, `LEDS_TRIGGER_TIMER` for every board, plus `GPIO_BCM_VIRT`
  for pi-3b specifically — the 3B's ACT LED hangs off
  `brcm,bcm2835-virtgpio` (the firmware's virtual GPIO over the mailbox),
  not the SoC GPIO. The 3B+ moved it to a real GPIO; the shared pi-3b image
  covers both, so the symbol is required.
- **No `internal/artifacts.Version` bump, and no artifacts release.** Every
  one of these symbols is already `=y` in the published v0.10.2
  kernel.config for its board, so the merged config — and therefore the
  compiled artifact — is unchanged. This PR only records the dependency.
- Do not "fix" a symbol by reading `build/boards/*/kernel.config`; check the
  fragment, or the published artifact.

## Todo

- [x] Add the symbols to all 8 board fragments
- [x] Confirm each symbol is already `=y` in that board's released config, so
      no artifact changes
- [x] Note in the PR why no artifacts release is needed

## Summary of Changes

Added `CONFIG_NEW_LEDS=y`, `CONFIG_LEDS_CLASS=y`, `CONFIG_LEDS_GPIO=y`,
`CONFIG_LEDS_TRIGGER_TIMER=y` to all 8 board fragments (pi-zero-w,
pi-zero-2w, pi-3b, cubie-a5e, nanopi-zero2, radxa-zero-3e, rock-4se,
qemu-virt), plus `CONFIG_GPIO_BCM_VIRT=y` to pi-3b only. Confirmed absent
from pi-zero-w and NOT added there per the bean's trap warning.

Verified every symbol against the extracted `kernel.config` from the
published `artifacts/v0.10.2` GitHub release (not the committed snapshots)
for all 8 boards: all four core symbols were already `=y` everywhere, and
`GPIO_BCM_VIRT` was already `=y` on pi-3b and genuinely absent from
pi-zero-w's release. So the merged config, and every compiled artifact, is
unchanged — no `internal/artifacts.Version` bump and no artifacts release.

For the Pi boards, `internal/kernelspec`'s `RequiredY` is mechanically
derived from the fragment's literal `=y` lines, so the new symbols are
picked up automatically (confirmed by `TestPiRequiredYIsDerivedFromFragment`
passing). The fleet boards' `RequiredY` in `internal/kernelspec/kernelspec.go`
is a hand-curated subset of board-identity-defining driver symbols that
already omits many generic `=y` fragment lines (e.g. `GPIO_CDEV`, `I2C`,
`SERIAL_8250`) — consistent with that existing pattern, the new generic LED
symbols were left out of those hand-maintained lists too; the fragment is
the assertion per the bean's locked decision.

All quality gates (`go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run ./...` on both darwin and linux) pass.
