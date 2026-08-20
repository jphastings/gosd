---
# gosd-ddz6
title: Log which status LED a board resolved to
status: completed
type: task
priority: normal
created_at: 2026-08-17T21:09:22Z
updated_at: 2026-08-20T04:32:41Z
---

The boot-state LED (gosd-xtcs) logs only on failure. It never says which LED
it selected, nor that it found none — and gosd-init has no shell, no SSH and
no remote debug, so the serial console is the only channel. On a board whose
LED does not behave, there is currently no way to tell apart:

- `/sys/class/leds` held no entries at all,
- entries existed but every one failed the `gpio-leds` candidate filter (a
  wrong `device/of_node/compatible` path would look exactly like this),
- the wrong LED was selected,
- the trigger/brightness writes failed.

Each guess otherwise costs a reflash cycle. This bean adds the one line that
distinguishes them, ahead of the first bench run (nanopi-zero2).

## Locked decisions

- **One line, logged once**, right after the first state is applied — that is
  the earliest point at which lazy discovery has actually run.
- It names the **selected** LED and the gpio-leds candidates it chose from;
  when nothing was selected it instead names the sysfs entries that were
  present but **rejected**, so a filter bug is visible immediately; and it
  distinguishes that from no registered LEDs at all.
- **Optional interface**, so no existing fake has to change and a nil
  StatusLED stays a silent no-op.
- Diagnostic only. It must not change which LED is chosen or any state write.

## Todo

- [x] Report candidates/rejected/selected out of discovery
- [x] Log it once from the boot sequence
- [x] Tests for the three shapes (selected, all rejected, none at all)

## Summary of Changes

`statusled.Inspect` is now the one place that scans and selects, returning a
`Discovery` with the selected LED, the gpio-leds candidates it chose from,
and the entries it rejected. `Discover` became a thin wrapper over it, so its
signature and every existing test are untouched, and the logged line can
never describe a different choice from the one actually made. Both lists are
sorted, since `os.ReadDir` order is unspecified and the line needs to read
the same on every boot.

`Sysfs.Describe` renders that through the same `sync.Once`-cached discovery
the state methods act on, and `boot.Run` logs it immediately after the
booting state, through a new optional `StatusLEDDescriber` — optional so no
existing fake had to change and a nil StatusLED stays a silent no-op.

What the console now says, one line per boot:

- `status LED: using green:status (gpio-leds candidates: green:status, red:heartbeat)`
- `status LED: no gpio-leds LED found; these entries are not gpio-leds backed: input0::capslock`
- `status LED: no LEDs registered at all`

The middle case is what earns this bean: it separates a wrong
`device/of_node/compatible` path from a board that genuinely has nothing —
otherwise indistinguishable, and a reflash cycle apart.

Selection and the state writes are unchanged.
