---
# gosd-xtcs
title: Show boot state on an onboard LED
status: in-progress
type: feature
created_at: 2026-08-17T20:10:28Z
updated_at: 2026-08-17T20:10:28Z
---

GoSD devices are headless. Today a board that is mid-boot, wedged, or running
happily are indistinguishable without a serial console. Every supported board
has at least one software-controllable onboard LED; this bean uses it as the
one signal a non-developer owner can read across the room.

## Locked decisions

- **Three states, nothing else.** Blink 250ms on / 250ms off = booting;
  blink 125/125 = a fatal error was recorded (see the crash-report docs);
  solid on = the app started and has been handed control.
- **The kernel does the blinking**, via the `timer` trigger's `delay_on` /
  `delay_off`. Not a goroutine. This is load-bearing: the fatal blink must
  survive `fault.Fatal` halting the board, and a wedged gosd-init must keep
  blinking "still booting" rather than freezing the LED.
- **LED selection order, applied to the board's own device tree:**
  1. `function` is `activity` or `status`, or `label` is `ACT`
  2. colour is `green`
  3. `function` is `power`, or `label` is `PWR`
  4. the board's only LED

  Ties inside a tier break to green first, then lexicographically by sysfs
  name, so the choice is stable across boots and kernel versions.
- **Only `gpio-leds` LEDs are candidates**, proven positively by the parent
  device's `of_node/compatible`. `CONFIG_INPUT_LEDS=y` on every board, so a
  plugged-in USB keyboard would otherwise offer `input0::capslock`; no board
  declares PHY LED subnodes today but a kernel bump could add them.
- **Power is deliberately NOT preferred first.** On pi-3b/3b+ the LED marked
  PWR is red, and Pi firmware uses it for the undervoltage warning — a
  diagnostic worth more than our status. Preferring activity keeps the whole
  Pi family on the green ACT LED and leaves PWR alone.
- **A board with no LED is a silent no-op**, not an error (qemu-virt has none,
  so CI cannot exercise the blink end-to-end).
- **No config-tree key.** Not user-configurable in this bean.

## What each board resolves to

Verified by dtc against the DTBs in the artifacts/v0.10.2 release.

| Board | Selected | Colour | Matched on |
|---|---|---|---|
| pi-zero-w | `ACT` | green | label |
| pi-zero-2w | `ACT` | green | label |
| pi-3b | `ACT` | green | label |
| pi-3b+ | `ACT` | green | label |
| radxa-zero-3e | `green:heartbeat` | green | colour (only LED) |
| rock-4se | `blue:status` | blue | function `status` |
| nanopi-zero2 | `green:status` | green | function `status` |
| cubie-a5e | `blue:activity` | blue | function `activity` |

`heartbeat` deliberately does not count as activity/status: that is what makes
nanopi-zero2 pick green over red, and it costs nothing on radxa-zero-3e where
the heartbeat LED is the only one.

## Documentation this must support

> The LED marked ACT, or the activity/status LED, or the green LED, or the
> only LED your board has — in that order, depending on what your board
> provides — blinks slowly while booting, blinks rapidly if a fatal error was
> recorded, and is solid on once your application is running.

Ship the resolved table too, so nobody runs the rule in their head.

## Todo

- [x] `cmd/gosd-init/internal/statusled`: discovery, selection, state writes
- [x] Wire the three states into the boot sequence
- [x] Tests against a fake sysfs root (must pass on macOS)
- [x] Docs page + link from the crash-report docs
- [x] COMPATIBILITY.md
- [ ] Bench verification on real hardware (unverifiable in CI)

## Summary of Changes

- Added `cmd/gosd-init/internal/statusled`: `Discover(root)` enumerates a
  sysfs LEDs directory (default `/sys/class/leds`, injectable for tests),
  filters to `gpio-leds`-backed candidates only (positively proven via
  `device/of_node/compatible`, which excludes an `input0::capslock`-style
  LED), parses each candidate's sysfs name into label/colour/function, and
  applies the four-tier selection rule with deterministic tie-breaking
  (green first, then lexicographic sysfs name) so directory order never
  decides the outcome. `LED`'s `Booting`/`Fatal`/`Running` methods write the
  kernel's `timer` trigger (claiming it before `delay_on`/`delay_off`,
  which only exist once it's active) or `none` + `max_brightness`, per the
  locked write order. `Sysfs` is the real `boot.StatusLED` implementation:
  it discovers lazily (on first use, since `/sys` isn't mounted until
  `gosd-init`'s own early mounts run) and caches the result.
- Added a `StatusLED` interface and `Deps.StatusLED` field to
  `cmd/gosd-init/internal/boot`. Wired three call sites: `Booting` right
  after the console opens, `Running` once the first `/app` start succeeds
  (not on every later restart — there's no fourth "restarting" state),
  and `Fatal` on every path that actually halts the device (`fatal()`'s
  `class.halt` branch and `haltForAppFault`) — deliberately not on a
  rebooting fatal (`GOSD-EARLY-MOUNT`/`GOSD-BOOT-MOUNT`), which is back to
  a fresh "booting" blink within 5s regardless. A nil `Deps.StatusLED` is a
  silent no-op, so qemu-virt and every pre-existing boot test needed no
  changes. Wired the real `statusled.New(statusled.DefaultRoot)` in
  `cmd/gosd-init/main.go`.
- Tests: `statusled` package covers the pi-3b/nanopi-zero2/cubie-a5e
  per-board shapes from the bean's table, the `input0::capslock` exclusion,
  no-LEDs-found, order-independence and tie-breaking, and the exact
  Booting/Running write order (via a swappable `writeFile` seam, since a
  plain temp dir can't reproduce the real "delay_on doesn't exist until
  timer is active" failure). `boot` package tests cover the Booting→Running
  sequencing, the halt-only Fatal scope (both call sites), and that a
  rebooting fatal does not fire it. All pass on macOS with no build tags.
- Docs: added `docs/status-led.md` (the three states, the selection rule,
  the resolved per-board table, the required user-facing sentence verbatim);
  linked it from `docs/crash-reports.md` (the fast blink points there) and
  from `README.md`'s feature list; added a `COMPATIBILITY.md` bullet noting
  it's code-complete/unit-tested but not yet bench-verified.
- Everything here is unit- and QEMU-tested only; qemu-virt has no LED, so
  end-to-end blink behaviour has never been observed on a real board. The
  bean's own per-board table was pre-verified by `dtc` against real DTBs, so
  the selection *logic* should hold, but the actual kernel `timer` trigger
  and the write-order constraint it's built around are untested outside the
  code's own reasoning about how sysfs behaves. No locked decision proved
  wrong during implementation.
