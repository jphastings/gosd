---
# gosd-qfgl
title: sound.Open picks a Loopback card and silently plays into nothing
status: completed
type: bug
priority: high
created_at: 2026-07-30T21:48:51Z
updated_at: 2026-07-30T22:56:00Z
---

## Symptom

On a ROCK 4SE whose kernel has `CONFIG_SND_ALOOP=y`, `examples/chime` — asked
explicitly for HDMI — opened the loopback instead and played to a device that
discards everything:

```
chime: boot chime, then a test tone every 10s (override with CHIME_EVERY)
playing to Loopback PCM (/dev/snd/pcmC0D0p) at 48000 Hz, 2 channels
card 0: the audibility pass changed nothing
```

`CHIME_OUTPUT=hdmi` was applied (`[gosd] app env: CHIME_EVERY, CHIME_OUTPUT`),
so `Options.Prefer = sound.HDMI` was set and did not prevent this. The card
list was:

```
ALSA device list:
  #0: Loopback 1
  #1: Analog
  #2: hdmi-sound
```

The audibility pass correctly "changed nothing" — a loopback has no volume or
mute to set — so there is no error, no warning, and no sound. The app looks
healthy.

## Why it matters more than it looks

snd-aloop is a **virtual** device: it is never a real output, and playing to
it is always wrong. Because it registers as card 0 it wins any search that
starts at the lowest card, and it silently swallows the audio.

In this session it swallowed audio from **two independent stacks** before it
was spotted: mpv's `--ao=alsa` (which opens ALSA's `default`, also card 0),
and then gosd's own `sound` package. It cost hours: the mpv silence was first
misdiagnosed as a format problem, then as a plug-conversion problem, and a
separate HDMI "static" symptom was chased through DTB inspection and pixel
clocks before the loopback was noticed at all.

GoSD's own fragments already deny it — `# CONFIG_SND_ALOOP is not set` is in
`rock-4se-analog`, `rock-4se-hdmi` and `radxa-zero-3e-hdmi` — so a
by-the-book image is safe. But `docs/sound.md` explicitly invites apps to
write their own recipes, and the deny-list section documents exactly how easy
it is for defconfig promotion to pull in drivers nobody asked for. An app with
a hand-rolled fragment (which is how this was found) gets a loopback and no
diagnosis. The library should not depend on every downstream fragment being
perfect.

## Proposed approach (not locked)

1. **Skip loopback cards in the device search.** They are never a valid
   playback target. Match on the card/driver identity rather than a substring
   of a user-visible string if possible — `snd-aloop`'s card id is stable.
   Note `control.go` already has "loopback" in `captureWords`, but that governs
   *mixer element* matching, not *device* selection, so it does not help here.
2. **Make `Options.Prefer` authoritative when it can be satisfied.** Asking for
   `HDMI` and silently receiving a loopback is the shape of the bug; if a
   preferred output exists, it should win outright.
3. **Say something when the only device found is unusual.** Even after (1),
   an app that ends up on a surprising device deserves a log line naming it —
   `Device.Name()` already carries the information, the caller just has no
   reason to suspect it.
4. Consider whether `Options.Path` should be the documented escape hatch in
   `docs/sound.md`'s gotchas, alongside a new entry for this — it is what
   unblocked us (`CHIME_DEVICE=/dev/snd/pcmC2D0p`).

## Acceptance

- On a card list containing a Loopback, `sound.Open()` and
  `OpenWith(Options{Prefer: HDMI})` both select a real output, not card 0.
- A board with ONLY a loopback reports `ErrNoDevice` rather than pretending to
  play.
- Unit-testable on macOS against the existing fakes; no hardware needed for
  the selection logic.
- `docs/sound.md` gains a gotcha entry — this is a trap any app with its own
  kernel recipe can fall into.

## Summary of Changes

**A virtual card is recognised by driver identity, not by its name.**
Discovery now reads `/proc/asound/cards` alongside `/proc/asound/pcm` and
joins each PCM to its card's `id`/`driver`/`shortname`. A card whose driver
(or id) is `Loopback` or `Dummy` is a `virtualCard` — those strings are
literals in `sound/drivers/aloop.c` and `dummy.c`, so they are stable, unlike
the user-visible longname (`Loopback 1`, which counts cards) or the id (a
module parameter can override it). `snd-dummy` is included because it is the
same trap under another name; nothing else was added speculatively.

**Virtual cards are removed from the search entirely, not deprioritised.**
`playable()` splits the PCM list into candidates and virtual devices before
ranking, so `Open()` and `OpenWith(Options{Prefer: HDMI})` both land on a real
output on the ROCK 4SE list from the symptom above. When a board has *only*
virtual cards, `virtualOnlyError` wraps `ErrNoDevice` and names the PCMs, the
modules and the Kconfig lines to deny (`# CONFIG_SND_ALOOP is not set`), plus
`Options.Path` as the deliberate way in. `Options.Path` still opens whatever
PCM it names — it bypasses discovery — which keeps the escape hatch that
unblocked the bench session (`CHIME_DEVICE=/dev/snd/pcmC2D0p`).

**`Prefer` can now actually see a Rockchip HDMI sink, which is the other half
of the symptom.** `isHDMI()` only looked at the PCM's own id and name, and an
ASoC PCM is named after its DAI link — the ROCK 4SE's HDMI PCM reads
`ff8a0000.i2s-i2s-hifi i2s-hifi-0`, with no "hdmi" anywhere. `Prefer: HDMI`
therefore matched *nothing*, ranking collapsed to card order, and card 0's
loopback won: that is how an explicit HDMI request opened a loopback without
any device failing to open. `isHDMI()` now also matches the card's id, driver
and name (`hdmisound` / `hdmi-sound`), which the same `/proc/asound/cards`
read makes available.

**Surprises are said out loud: new `Options.Logf func(format string, ...any)`.**
Set it (`examples/chime` passes `log.Printf`) and Open reports each virtual
card it skipped, and any preference it could not honour — either "this board
exposes no HDMI playback device" or "no HDMI playback device would open
(<the errors>), so playing to <device> instead". Nothing is logged when Open
does exactly what was asked, and nothing is logged during playback. This is
the deliberate departure from proposal 2: rather than making `Prefer` a hard
constraint, the fallback across candidates is kept (a board whose first choice
is busy still gets sound) and made *audible in the log*, since the failure
being fixed is silence-without-explanation, not the fallback itself. With the
`isHDMI()` fix a preferred output that exists and opens now genuinely wins.

Selection is pure and fixture-driven: `sound/sound_test.go` runs the real ROCK
4SE `/proc/asound/pcm` + `/proc/asound/cards` text through the whole pipeline
on macOS — loopback never a candidate for Any/HDMI/Analog, HDMI found only via
its card, a loopback+dummy-only board reporting `ErrNoDevice`, and the notice
text.

Files: `sound/sound.go`, `sound/platform_linux.go`, `sound/sound_test.go`,
`docs/sound.md` (new gotcha + `Options.Logf` guidance),
`examples/chime/main.go`, `examples/chime/README.md`.

## Bench verification (still to do)

- [ ] Re-run `examples/chime` on the ROCK 4SE with the loopback-enabled kernel
      that found this: expect a log line naming the skipped `snd-aloop` card,
      playback on card 1 (jack) or card 2 (HDMI) rather than card 0, and an
      audible tone with no `CHIME_DEVICE` override.
