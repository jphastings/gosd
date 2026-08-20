---
# gosd-54j8
title: Retain the status LED level through kernel shutdown on every board
status: in-progress
type: task
priority: normal
created_at: 2026-08-18T12:24:43Z
updated_at: 2026-08-20T00:11:27Z
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

- [x] DTS patches: Rockchip (radxa-zero-3e, rock-4se, nanopi-zero2)
- [x] DTS patch: Allwinner (cubie-a5e)
- [x] DTS patches: Pi (pi-zero-w, pi-zero-2w, pi-3b)
- [x] `artifacts:` change file, no Version bump
- [ ] Bench: one board per family, pi-3b explicitly
- [ ] Follow-up PR bumping `internal/artifacts.Version`



## Summary of Changes

Added a DTS patch to every board's status-LED node adding
`retain-state-shutdown;` and setting `default-state = "off";` (added where
absent, flipped where it was `"on"`):

- pi-zero-w: `&led_act` in `bcm2835-rpi-zero-w.dts` (had neither property —
  inherited `default-state = "keep"` from `bcm283x-rpi-led-deprecated.dtsi`)
- pi-zero-2w: `&led_act` in `bcm2710-rpi-zero-2-w.dts` (default-state was
  already `"off"`)
- pi-3b: `&led_act` in both `bcm2710-rpi-3-b.dts` and
  `bcm2710-rpi-3-b-plus.dts` (one combined patch file; default-state was
  already `"off"` in both)
- radxa-zero-3e: `led-green` in the shared `rk3566-radxa-zero-3.dtsi`
  (default-state was `"on"`)
- rock-4se: `led-0` in the shared `rk3399-rock-pi-4.dtsi` (had no
  default-state)
- nanopi-zero2: `led-1` (`green:status`, not `led-0` the red heartbeat LED)
  in `rk3528-nanopi-zero2.dts` (default-state was `"on"`)
- cubie-a5e: `use-led` (`blue:activity`, not `power-led`) in
  `sun55i-a527-cubie-a5e.dts` (had no default-state)

Every patch was verified against the pinned kernel sources pre-fetched to
`/Users/jp/src/personal/gosd-bench-led/dts/` with both `patch -p1 --dry-run`
and a real apply diffed byte-for-byte against the intended edit — all seven
applied cleanly with no fuzz, using the same `patch -p1 --forward` invocation
`internal/kernelbuild` runs.

Plumbing: pi-3b and pi-zero-2w had no `kernel/patches/` directory or
`PatchesFS` before this bean — added both (mirroring pi-zero-w), and wired
`DTSPatches: loadPatches(...)` into their `internal/kernelspec` entries.
`kernelspec_test.go`'s `TestDTSPatchesOnlyOnExpectedBoards` allowlist and
doc comment updated for the two newly-patched boards; no other
board-enumerating list needed a change.

Docs: rewrote `docs/status-led.md`'s "Known limitation" block, which said no
board shipped `retain-state-shutdown` and the LED went dark on halt — no
longer true. README.md and COMPATIBILITY.md needed no changes (neither made
that specific claim; COMPATIBILITY.md's "not yet bench-verified" note for
the status-LED feature as a whole, bean gosd-xtcs, still holds).

Shipped `.changeset/led-retain-state-shutdown.md` as `artifacts: minor`, per
docs/artifacts.md's tag-first, bump-second rule.
`internal/artifacts.Version` is untouched (still `v0.10.2`) — a follow-up PR
bumps it once the release carrying these patches exists.

All quality gates pass: `go build ./...`, `go test ./...`, `go vet ./...`,
`gofmt -l .` (clean), `golangci-lint run ./...` and
`GOOS=linux golangci-lint run ./...` (0 issues both).
