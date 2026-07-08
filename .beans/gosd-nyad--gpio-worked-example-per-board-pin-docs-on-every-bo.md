---
# gosd-nyad
title: 'GPIO: worked example + per-board pin docs on every board'
status: todo
type: feature
created_at: 2026-07-08T03:35:00Z
updated_at: 2026-07-08T03:35:00Z
parent: gosd-jge2
---

Make GPIO usable and documented on all four boards. CONFIG_GPIO_CDEV is already =y everywhere, so /dev/gpiochipN appears at boot — this is a docs + worked-example job with NO kernel/DTB/config.txt change and NO artifact release.

LOCKED DECISIONS:
- Example uses github.com/warthog618/go-gpiocdev (the modern /dev/gpiochip chardev API; pure Go, CGO-free, works on arm64 AND arm/GOARM=6 — verify the GOARM=6 cross-compile in CI). This is the library docs already recommend; adding it to go.mod is intended.
- SAFE BY DEFAULT (mirror examples/i2cscan politeness): the example must not drive arbitrary output pins on unknown wiring. Default behaviour = enumerate: open each /dev/gpiochipN, print chip name/label/line count and each line’s name/consumer/direction (a gpioinfo-style dump), read-only. OPT-IN blink: if env GOSD_GPIO_CHIP + GOSD_GPIO_LINE are set, request that one line as output and toggle it a few times, logging each step. Never drives anything unless explicitly told which line.
- examples/gpioinfo (or gpiodemo) as the dir name — your call; stdlib + go-gpiocdev only; add to whatever CI example-build list exists.

Per-board work:
- [ ] examples/gpioinfo: enumerate (default) + opt-in blink via env; compiles for arm64 and GOARM=6 in CI
- [ ] docs/runtime.md GPIO section: per-board gpiochip numbering (which chip backs the 40-pin/FPC header — read the DTS/gpio labels at v6.18.37), header-pin -> (chip,line) mapping for each board (incl. NanoPi FPC pin numbers), go-gpiocdev + periph.io pointers, a note that BCM GPIO numbers != physical pin numbers != gpiochip line offsets
- [ ] COMPATIBILITY.md: split the current "GPIO / SPI" row — GPIO becomes its own row, ✅ all four (code+docs complete, bench-pending); SPI stays its own 🚧 row (its bean follows)
- [ ] Bench: real LED blink on each board (leave unchecked)

## Acceptance
Example compiles both arches in CI and runs (enumerates) under `gosd run` on qemu-virt (gpiochip present via virtio? if not, note that qemu has no GPIO and the enumerate path degrades gracefully — test that graceful path). Docs give a correct (chip,line) for at least one header pin per board.
