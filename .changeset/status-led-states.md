---
gosd: minor
---

#### The status LED's running and fatal states have changed

v0.6.4 introduced the status LED with a fast blink for a recorded fatal
error. On real hardware that signal does not exist: `gosd-init` halts the
board immediately after recording the error, and a halted kernel stops
driving the LED, so the fast blink lasted about a tenth of a second and then
went dark. The release notes claimed it kept blinking through the halt. It
did not.

The three states are now:

| State | LED | Previously |
| --- | --- | --- |
| Booting | Flashes evenly, 250ms on/off | unchanged |
| Running | Blips briefly, 50ms on / 950ms off | solid on |
| Fatal | Solid on | fast 125ms blink |

Fatal is steady because a steady level is the only thing that can outlive the
halt. Running became a blip so that it stays clearly distinct from a solid
LED, and so a healthy board reads as alive rather than merely lit.

If you were relying on the previous meanings — solid for healthy, fast blink
for broken — those two have effectively swapped, and a device that has
halted is now the one showing a steady light.

One caveat worth knowing: for the fatal state to survive the halt at all, a
board's device tree has to mark the LED as retaining its state through
shutdown. No board ships that yet, so for now the LED still goes dark once
the device halts. Everything before the halt is unaffected, and the
[status LED guide](docs/status-led.md) tracks where this stands.
