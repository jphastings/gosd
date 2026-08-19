---
# gosd-54j8
title: Retain the status LED level through kernel shutdown on every board
status: todo
type: task
created_at: 2026-08-18T12:24:43Z
updated_at: 2026-08-18T12:24:43Z
---

The status LED's failure signal (solid on, bean gosd-n82u) only persists if
the board's device tree asks for it. `gpio_led_shutdown()` turns every GPIO
LED off during `device_shutdown()` unless the LED carries
`LED_RETAIN_AT_SHUTDOWN`, which comes from the DT property
`retain-state-shutdown`. Verified 2026-08-18: **no LED on any of the eight
boards sets it**, which is exactly why the bench saw the LED go dark on halt.

Verified the same day, against the released v0.10.2 artifacts: every kernel
we ship supports the property — both trees, including pi-zero-w's 32-bit
zImage (decompressed before grepping, or it false-negatives). So this is a
device-tree change only, with no kernel config work.

## Locked decisions

- Add `retain-state-shutdown` to the **selected** LED node on all eight
  boards. One coordinated artifacts release covers the fleet: the Pi DTBs are
  built from the rpi tree just as the Rockchip and Allwinner ones are, so the
  same DTS-patch mechanism works everywhere and no overlay machinery is
  needed. (Pi firmware *could* apply an overlay, but introducing a second
  mechanism for three boards is not worth it.)
- **Also set `default-state = "off"` on that same node**, in the same patch.
  Every board currently ships `default-state = "on"`, so solid-on is also
  what shows between `leds-gpio` probing and gosd-init claiming the LED — and
  what shows if gosd-init never runs at all. Flipping it makes solid-on mean
  failure and nothing else. Accepted trade-off: a board whose kernel boots but
  whose gosd-init dies immediately reads as dark rather than lit.
- Tag-first, bump-second, per docs/artifacts.md: ship the DTS patches with an
  `artifacts:` change file and NO `internal/artifacts.Version` bump; a
  follow-up PR bumps the pin once the release exists.

## Verify at the bench, not by reasoning

Every previous assumption in this area survived review and then failed on
hardware. Confirm on one board per family that the LED still shows its level
after `reboot: System halted`.

**pi-3b is the one to distrust.** Its ACT LED sits on
`brcm,bcm2835-virtgpio`, the firmware's mailbox GPIO, not the SoC GPIO. The
retain flag means we simply do not write at shutdown, which is the safer
path — but whether the *firmware* reasserts its own behaviour once Linux
halts is unknown. If it does not hold, pi-3b's failure signal degrades to
"off" and that gets recorded here rather than worked around.

## Todo

- [ ] DTS patches: Rockchip (radxa-zero-3e, rock-4se, nanopi-zero2)
- [ ] DTS patch: Allwinner (cubie-a5e)
- [ ] DTS patches: Pi (pi-zero-w, pi-zero-2w, pi-3b)
- [ ] `artifacts:` change file, no Version bump
- [ ] Bench: one board per family, pi-3b explicitly
- [ ] Follow-up PR bumping `internal/artifacts.Version`
