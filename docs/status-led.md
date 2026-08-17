# Status LED: showing boot state without a screen

A GoSD device is headless: no screen, no shell, no SSH. Without a serial
cable attached, there's no way to tell "still booting" from "wedged" from
"running fine" — which matters most to the person least likely to have a
cable, the one who just plugged the device in.

Every supported board has at least one software-controllable onboard LED,
and `gosd-init` uses it as that one signal:

> The LED marked ACT, or the activity/status LED, or the green LED, or the
> only LED your board has — in that order, depending on what your board
> provides — blinks slowly while booting, blinks rapidly if a fatal error
> was recorded, and is solid on once your application is running.

This is automatic. There's no flag to turn it on, and nothing in your app
needs to change.

## The three states

| State | LED behaviour | When |
| --- | --- | --- |
| Booting | Blinks at 250ms on / 250ms off | From just after `gosd-init` opens the console, until your app starts |
| Fatal | Blinks at 125ms on / 125ms off — twice as fast | A fatal error was recorded and the device has halted — see [the crash-report guide](crash-reports.md) |
| Running | Solid on | `/app` has started successfully and been handed control |

Once your app has started, an ordinary crash-and-restart (the common case —
see "What you get for free" in the crash-report guide) does not blink the
LED back to booting; the LED only ever moves to the fast fatal blink, which
happens on a **halt**, not a restart. There is no fourth "restarting" state.

**The blink is driven by the kernel, not by `gosd-init` itself.** Both
blinking states claim the LED's `timer` trigger and set its `delay_on`/
`delay_off` files; the kernel does the actual blinking from then on. This is
deliberate: the fatal blink has to keep going after the board halts (nothing
in userspace is running to blink it), and a wedged `gosd-init` — the exact
situation "still booting" needs to show — can't be trusted to keep a
goroutine alive either.

## Which LED gets used

Boards ship anywhere from one onboard LED to several, not all of them
useful as a status indicator — some are wired to a different purpose
entirely by the kernel's default trigger (an SD-card activity light, for
instance). `gosd-init` picks one, in this order, and only ever considers an
LED whose device-tree parent identifies it as a plain GPIO LED (which is
what excludes, for example, a plugged-in USB keyboard's caps-lock LED):

1. The LED whose function is `activity` or `status`, or whose label is
   `ACT`.
2. The LED whose colour is `green`.
3. The LED whose function is `power`, or whose label is `PWR` or `POWER`.
4. Whatever LED is left — typically the board's only one.

A `heartbeat` LED deliberately doesn't count as activity/status: it's a
"the kernel is alive" indicator with its own default blink, not a status
LED gosd-init should claim.

**Power is deliberately not preferred first.** On the Raspberry Pi 3B/3B+,
the LED marked PWR is red and the firmware uses it for its own undervoltage
warning — a diagnostic worth more than boot status, and left alone by
preferring the green ACT LED instead.

If more than one LED matches the same tier, the tie breaks to the green one,
then to whichever sysfs name sorts first — so the choice is stable across
boots and kernel versions, never left to directory-listing order.

A board with no onboard LED at all (the internal `qemu-virt` profile, used
for CI) is a silent no-op: nothing blinks, and nothing about boot changes.

## What each board resolves to

| Board | LED used | Colour | Matched on |
| --- | --- | --- | --- |
| Pi Zero W | `ACT` | green | label |
| Pi Zero 2W | `ACT` | green | label |
| Pi 3B / 3B+ | `ACT` | green | label |
| Radxa Zero 3E | `green:heartbeat` | green | colour (only LED) |
| ROCK 4SE | `blue:status` | blue | function `status` |
| NanoPi Zero2 | `green:status` | green | function `status` |
| Radxa Cubie A5E | `blue:activity` | blue | function `activity` |

## Not configurable

There's no config-tree key or build flag for this: which LED gets used, and
what each state looks like, follows directly from the board's own device
tree. That keeps the one universal, always-on signal simple enough to trust
without reading a manual.
