---
# gosd-nyad
title: 'GPIO: worked example + per-board pin docs on every board'
status: in-progress
type: feature
priority: normal
created_at: 2026-07-08T03:35:00Z
updated_at: 2026-07-08T06:08:34Z
parent: gosd-jge2
---

Make GPIO usable and documented on all four boards. CONFIG_GPIO_CDEV is already =y everywhere, so /dev/gpiochipN appears at boot — this is a docs + worked-example job with NO kernel/DTB/config.txt change and NO artifact release.

LOCKED DECISIONS:
- Example uses github.com/warthog618/go-gpiocdev (the modern /dev/gpiochip chardev API; pure Go, CGO-free, works on arm64 AND arm/GOARM=6 — verify the GOARM=6 cross-compile in CI). This is the library docs already recommend; adding it to go.mod is intended.
- SAFE BY DEFAULT (mirror examples/i2cscan politeness): the example must not drive arbitrary output pins on unknown wiring. Default behaviour = enumerate: open each /dev/gpiochipN, print chip name/label/line count and each line’s name/consumer/direction (a gpioinfo-style dump), read-only. OPT-IN blink: if env GOSD_GPIO_CHIP + GOSD_GPIO_LINE are set, request that one line as output and toggle it a few times, logging each step. Never drives anything unless explicitly told which line.
- examples/gpioinfo (or gpiodemo) as the dir name — your call; stdlib + go-gpiocdev only; add to whatever CI example-build list exists.

Per-board work:
- [x] examples/gpioinfo: enumerate (default) + opt-in blink via env; compiles for arm64 and GOARM=6 in CI
- [x] docs/runtime.md GPIO section: per-board gpiochip numbering (which chip backs the 40-pin/FPC header — read the DTS/gpio labels at v6.18.37), header-pin -> (chip,line) mapping for each board (incl. NanoPi FPC pin numbers), go-gpiocdev + periph.io pointers, a note that BCM GPIO numbers != physical pin numbers != gpiochip line offsets
- [x] COMPATIBILITY.md: split the current "GPIO / SPI" row — GPIO becomes its own row, ✅ all four (code+docs complete, bench-pending); SPI stays its own 🚧 row (its bean follows)
- [ ] Bench: real LED blink on each board (leave unchecked)

## Acceptance
Example compiles both arches in CI and runs (enumerates) under `gosd run` on qemu-virt (gpiochip present via virtio? if not, note that qemu has no GPIO and the enumerate path degrades gracefully — test that graceful path). Docs give a correct (chip,line) for at least one header pin per board.


## Summary of Changes

GPIO is now documented and has a safe, cross-board worked example — code and
docs complete on all four boards; the one unchecked item (bench: a real LED
blink on hardware) is out of scope for this bean by design.

- **`examples/gpioinfo`**: default mode opens every `/dev/gpiochipN` present
  and prints a `gpioinfo`(1)-style dump (chip name/label/line count, then
  each line's offset/name/direction/consumer) - entirely read-only. Opt-in
  blink only when both `GOSD_GPIO_CHIP` and `GOSD_GPIO_LINE` are set: that
  one line is requested as an output and toggled 6 times (3 on/off cycles),
  logging each transition, then reverted to input on exit - mirrors
  `examples/i2cscan`'s "never touch anything not explicitly named" politeness.
  Uses `github.com/warthog618/go-gpiocdev` v0.9.1 (the modern chardev API),
  no other new dependency. The package carries a `//go:build linux` tag: the
  library's own `uapi` package is itself linux-only (build-tagged in its
  source), so the example can't build for any other GOOS - this is expected
  and matches the "Linux-only runtime code goes behind build tags" locked
  decision.
- **Cross-compile verified directly** (not just via CI): both
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` and
  `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6` build the example cleanly.
  go-gpiocdev is pure Go, chardev-ioctl based (via `golang.org/x/sys/unix`),
  so GOARM=6 was never expected to be a problem, and isn't. Added to CI's
  smoke-build job's arm64 and armv6 cross-compile lists alongside
  `examples/i2cscan`.
- **Real bug found and fixed while verifying `gosd run`**:
  `internal/build.requireMainPackage` ran `go list` under the *host's* GOOS
  rather than the target `linux` GOOS `CrossCompile` actually builds for. On
  a macOS host this rejected `examples/gpioinfo` outright ("build
  constraints exclude all Go files") even though it's a perfectly valid
  `package main` under the Linux build gosd performs - any Linux-only-tagged
  main package would have hit this on a non-Linux host. Fixed by forcing
  `GOOS=linux` on the preflight `go list` call too (`internal/build/build.go`),
  with a regression test (`TestCrossCompileRecognizesLinuxOnlyMainPackage`,
  `internal/build/testdata/linuxonly`) mirroring the exact shape of the bug.
- **Per-board gpiochip numbering** (`docs/runtime.md`'s new "GPIO is
  available via /dev/gpiochipN" section), verified against kernel source at
  the pinned tag `v6.18.37` (both boards' Rockchip findings cross-checked
  against `rk3568-pinctrl.dtsi`/`rk3528-pinctrl.dtsi`'s actual
  `i2c3m0_xfer`/`i2c5m0_xfer` pinctrl definitions, and against
  `gpio-rockchip.c`/`gpiolib.c`'s registration-order chip-numbering, not
  assumed):
  - **Pi Zero 2W / Zero W**: a single `gpiochip0` (54 lines) covers the whole
    SoC - confirmed neither board's device-tree include chain has any
    second GPIO chardev (no `raspberrypi,firmware-gpio` expander on either
    board), and `gpio-ranges = <&gpio 0 0 54>` is an identity map, so
    `gpiochip0` line N == BCM GPIOn always, on both boards. Header pin 3
    (BCM GPIO2) -> `gpiochip0` line 2; pin 5 (GPIO3) -> line 3.
  - **Radxa Zero 3E** (RK3566): 5 independently-numbered banks,
    `gpiochip0`-`gpiochip4`, bank index == chip index (confirmed via
    `rk356x-base.dtsi`'s gpio-bank node order + no earlier-registering gpio
    chip on this board). Header pin 3 (`GPIO1_A0`) -> `gpiochip1` line 0;
    pin 5 (`GPIO1_A1`) -> `gpiochip1` line 1.
  - **NanoPi Zero2** (RK3528): same 5-bank convention. FPC pin 12
    (`GPIO1_B2`) -> `gpiochip1` line 10; FPC pin 13 (`GPIO1_B3`) -> `gpiochip1`
    line 11.
  - Line-offset formula confirmed against
    `include/dt-bindings/pinctrl/rockchip.h`: `GPIOx_yN` = bank `x`,
    line-within-chip `group(y)*8 + N` (A=0,B=1,C=2,D=3).
  - Docs also carry the caution that BCM-number == gpiochip-line is a Pi-only
    coincidence of that DT's identity `gpio-ranges`, not a general rule, and
    that physical pin number is a third, independent numbering on every
    board.
- **`COMPATIBILITY.md`**: split the combined "GPIO / SPI" row into "GPIO"
  (now marked done for all four boards, code+docs complete/bench-pending,
  same fleet-wide "not hardware-verified" caveat as every other row) and
  "SPI" (still marked in-progress, tracked by bean `gosd-fnza`). Corrected the old
  `[^gpio]` footnote (previously described both GPIO and SPI as entirely
  unstarted) and the I2C footnote's stale "see the row below" reference,
  which predated the row split.

## qemu-virt verification (2026-07-08)

Ran `go run ./cmd/gosd run ./examples/gpioinfo --hostname gpio-test` (real
artifacts, `artifacts/v0.3.0`, downloaded fresh) and watched the serial
console.

**Deviation from the acceptance criterion's assumption**: qemu-virt's
`-M virt` machine model actually DOES emulate a GPIO controller - a PL061
(`gpiochip0 - 9030000.pl061`, 8 lines), with line 3 already claimed by the
kernel's own `gpio-keys` node ("GPIO Key Poweroff"). So the enumerate path
was exercised against a real (if virtual) device, not the empty/no-gpiochip
branch the acceptance criterion anticipated as the likely qemu outcome.
Observed output, repeated on every supervisor restart (the example exits 0
immediately in enumerate-only mode, so `gosd-init` dutifully restarts it
forever per its normal no-restart-limit policy - expected, not a bug):

```
gpiochip0 - 9030000.pl061 (8 lines):
  line   0: "unnamed"            input      unused
  line   1: "unnamed"            input      unused
  line   2: "unnamed"            input      unused
  line   3: "unnamed"            input      used by "GPIO Key Poweroff"
  line   4: "unnamed"            input      unused
  line   5: "unnamed"            input      unused
  line   6: "unnamed"            input      unused
  line   7: "unnamed"            input      unused
[gosd] /app (pid NNN) exited with status 0 after ~55ms
```

The truly-empty `gpiocdev.Chips() == nil` branch (the "no GPIO character
devices found" message) wasn't exercised end-to-end under qemu-virt as a
result - qemu's virt machine always provides this PL061, so there's no way
to get a genuinely gpiochip-less boot with the current fixed machine model.
That branch is a 3-line, directly-inspectable code path
(`main()`'s `len(chips) == 0` check), so it's covered by code review rather
than an end-to-end qemu run. Noting this here per the "if a locked
decision/acceptance assumption proves wrong in practice, say so" instruction
rather than silently treating the PL061 case as if it were the empty case.
