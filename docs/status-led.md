# Status LED: showing boot state without a screen

A GoSD device is headless: no screen, no shell, no SSH. Without a serial
cable attached, there's no way to tell "still booting" from "wedged" from
"running fine" — which matters most to the person least likely to have a
cable, the one who just plugged the device in.

Every supported board has at least one software-controllable onboard LED,
and `gosd-init` uses it as that one signal:

> The LED marked ACT, or the activity/status LED, or the green LED, or the
> only LED your board has — in that order, depending on what your board
> provides — flashes evenly while booting, blips briefly once a second while
> your application runs, and goes solid on if a fatal error was recorded.

This is automatic. There's no flag to turn it on, and nothing in your app
needs to change — unless your app wants to hold "booting" a little longer
itself, which is what [the ready package](#holding-booting-until-your-app-is-genuinely-ready)
below is for.

## The three states

| State | LED behaviour | When |
| --- | --- | --- |
| Booting | Flashes evenly, 250ms on / 250ms off | From just after `gosd-init` opens the console, until your app is handed control (see below) |
| Running | Blips briefly, 50ms on / 950ms off | By default, the instant `/app` starts successfully; an app that imports `github.com/jphastings/gosd/ready` instead holds Booting until it calls `ready.Signal()` |
| Fatal | Solid on | A fatal error was recorded and the device has halted — see [the crash-report guide](crash-reports.md) |

Booting and running are told apart by shape rather than speed alone: an even
flash against a short blip on a mostly-dark LED. Once your app has started,
an ordinary crash-and-restart (the common case — see "What you get for free"
in the crash-report guide) does not return the LED to booting; the LED only
moves to the fatal state on a **halt**, not a restart. There is no fourth
"restarting" state.

## Holding "booting" until your app is genuinely ready

By default `gosd-init` flips the LED to Running the instant `/app`'s process
starts — before your `main` has run a single line. That's fine for most
apps, but not for one that needs WiFi, an external API, or a sensor
initialized before it's genuinely "all okay": the LED would say Running
while your app is still failing every request.

Importing `github.com/jphastings/gosd/ready` opts an app out of that
default: `gosd build` inspects your app's own dependency graph and, when it
includes that package, holds the LED on Booting past process start. The LED
only moves to Running once your app calls `ready.Signal()` — call it as soon
as your app considers itself ready, however it defines that.

There's no separate flag for this and no timeout: an app that imports the
package and never calls `Signal` leaves the LED honestly stuck on Booting
rather than lying that it's running, and `gosd-init` logs that explicitly at
the first app start (its only way to warn you, since it has no shell or
remote debug). An app that wants a solid LED plus a written explanation on
the card instead calls `fault.Fatal` — see
[the crash-report guide](crash-reports.md). Full API docs are on the `ready`
package itself.

**The two blinking states are driven by the kernel, not by `gosd-init`
itself.** Both claim the LED's `timer` trigger and set its `delay_on` /
`delay_off` files; the kernel blinks it from then on. That matters for
booting in particular: a wedged `gosd-init` — the exact situation "still
booting" needs to show — can't be trusted to keep a goroutine alive, but a
kernel timer carries on regardless.

**Fatal is steady precisely because it can't be a blink.** `gosd-init` halts
the board immediately after recording a fatal error, and a halted kernel
stops the timer trigger dead, so a blinking fatal signal would last a
fraction of a second and then vanish. A steady level is the only thing that
can outlive the halt.

For that steady level to actually survive the halt, the board's device tree
has to mark the LED as retaining its state through shutdown — otherwise the
kernel turns every GPIO LED off on its way down. Every board's status LED
node carries that marker (`retain-state-shutdown`), so `gpio_led_shutdown()`
leaves the fatal report's solid-on level alone rather than clearing it. The
same device-tree change also flips the LED's power-on default to off on
every board, so solid-on can only mean the fatal state above — never
whatever level the GPIO happens to come up in before `gosd-init` claims the
LED, and never what shows if `gosd-init` never runs at all. One board's LED
sits behind firmware rather than being driven by Linux directly — the
Raspberry Pi 3B/3B+'s ACT LED is on the VideoCore's mailbox GPIO, not the
SoC's own — so whether its retained level survives past Linux's halt depends
on that firmware too, and needs confirming per board at the bench rather
than assumed from the device tree alone.

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
