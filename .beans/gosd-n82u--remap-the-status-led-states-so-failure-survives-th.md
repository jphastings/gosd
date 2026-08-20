---
# gosd-n82u
title: Remap the status LED states so failure survives the halt
status: completed
type: task
priority: normal
created_at: 2026-08-18T12:24:23Z
updated_at: 2026-08-20T04:32:51Z
---

Bench-proven on nanopi-zero2 (2026-08-18): the fatal state's fast blink is
invisible. `fault.Fatal` halts the board, and a halted kernel cannot blink —
`reboot: System halted` runs `device_shutdown()`, whose `gpio_led_shutdown()`
turns every GPIO LED off. The console showed the fatal state being set ~100ms
before the halt, so the 125ms blink existed for about a tenth of a second.

This corrects one half of gosd-xtcs's locked justification. "A wedged
gosd-init keeps blinking" remains TRUE — userspace dying does not stop a
kernel timer. "The fatal blink survives the halt" was FALSE: the kernel
stopping is precisely what stops it. Blink and halt are mutually exclusive,
by construction.

The signal that survives a halt is a steady level, so failure becomes solid
on, and the three states are remapped to keep them distinguishable.

## Locked decisions

- **Booting: 250ms on / 250ms off.** Unchanged.
- **Running: 50ms on / 950ms off.** A short regular blip, replacing solid on.
  Also fixes a real complaint: a healthy board no longer looks inert, and the
  blip is unmistakable against the boot flash.
- **Failure: solid on**, replacing the 125ms blink.
- **Selection is NOT changed.** The rule already prefers `activity`/`status`
  above colour and power, which is what picks `blue:activity` over
  `green:power` on cubie-a5e — the only board where that choice is live.
  Preferring RED was considered and rejected: exactly one board (nanopi-zero2)
  declares a red LED, the Pi boards carry no `color` property at all, and on
  pi-3b the red one is `PWR`, the firmware's undervoltage indicator. A colour
  rule is also undocumentable, since the answer differs per board.
- **This lands without an artifacts release and does not wait for one.** Until
  the DT work (see the sibling bean) ships `retain-state-shutdown`, the LED
  still goes off at halt on every board — the same as today, so no regression
  and no ordering constraint between the two.

## Todo

- [x] Remap the three states in `statusled`
- [x] Tests for the new timings, including that failure is steady, not blinking
- [x] Docs: the three states, and that persistence after halt needs the DT flag
- [x] Change file — see the correction below

## Correction to a locked decision (2026-08-18)

This bean was written assuming the LED feature had not yet shipped, and that
its pending change file could simply be amended. It had: **v0.6.4 released it
on 2026-08-17**, complete with release notes claiming the fatal blink "keeps
blinking through a fatal halt". So this is a behaviour change to a released
feature, not a correction to an unreleased one, and it ships its own change
file saying plainly that the previous fatal signal never worked and that the
running and fatal meanings have effectively swapped.

## Summary of Changes

`statusled` now maps booting to an even 250/250 flash (unchanged), running to
a 50ms/950ms blip, and fatal to a steady level. `Fatal` claims the `none`
trigger and writes `max_brightness`; `Running` moved onto the `timer` trigger
that `Fatal` gave up. Selection is untouched.

Tests assert the new timings, that fatal writes no `delay_on`/`delay_off` at
all (a blinking fatal state dies with the kernel), and that no two states
drive the LED identically — the property that actually matters, since the
three have to be told apart by eye.

Docs, README and COMPATIBILITY carry the new mapping, and the status LED
guide records the known limitation: until bean gosd-54j8 ships
`retain-state-shutdown` in each board's device tree, the LED still goes dark
at halt, exactly as it does today.

Real-hardware verification of the remapped LED states themselves (fatal
reading as steady on, running as a visible blip) is tracked separately by
gosd-ftw7 and gosd-54j8, not by this bean — the code shipped and is unit- and
QEMU-tested, but is not yet bench-proven.
