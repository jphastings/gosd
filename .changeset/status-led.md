---
gosd: minor
---

#### Boot state shows on your board's onboard status LED

A GoSD device is headless, and until now there was no way to tell "still
booting" from "wedged" from "running fine" without a serial cable. Every
supported board has at least one software-controllable LED, and `gosd-init`
now uses it as that signal automatically — no code changes, no config:

- Blinks slowly (250ms on/off) while booting.
- Blinks fast (125ms on/off) if a fatal error was recorded — see
  `docs/crash-reports.md`.
- Solid on once your app has started and been handed control.

`gosd-init` picks the LED marked ACT, or the activity/status LED, or the
green LED, or whichever LED the board has, following its own device tree —
see `docs/status-led.md` for the full selection rule and the per-board
table. The blink itself is driven by the kernel's own `timer` trigger, not a
goroutine, so it keeps blinking through a fatal halt or a wedged `gosd-init`.
