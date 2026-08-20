---
artifacts: minor
---

#### The status LED's fatal signal now survives kernel shutdown

Every board's status LED device-tree node now carries
`retain-state-shutdown`, so `gpio_led_shutdown()` no longer clears the LED's
level during `device_shutdown()`. Without it, a fatal error's steady on
signal (see [the status LED docs](docs/status-led.md)) went dark the moment
the kernel halted — the same halt that made the signal necessary in the
first place.

The same patch also flips the LED's `default-state` to `"off"` wherever it
wasn't already, so solid-on can only mean a recorded fatal error, never
whatever level the GPIO comes up in before `gosd-init` claims the LED.

One board's ACT LED sits on the Raspberry Pi 3B/3B+'s firmware-owned mailbox
GPIO rather than the SoC's own, so whether its retained level survives past
Linux's own halt depends on the firmware too — bench verification on real
hardware is tracked separately per board.
