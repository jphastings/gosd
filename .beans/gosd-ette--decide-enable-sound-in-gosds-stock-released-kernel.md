---
# gosd-ette
title: 'Decide: enable sound in GoSD''s stock released kernels (Route B)'
status: completed
type: task
priority: normal
created_at: 2026-07-29T21:45:24Z
updated_at: 2026-08-21T04:41:04Z
parent: gosd-qkbl
---

JP's call. See parent epic gosd-qkbl for the measured size evidence and the recommendation (Route A).


## Decision (JP, 2026-08-21): Route A — sound stays an opt-in recipe

**Audio stays a `gosd build-kernel` recipe. It does not go into GoSD's stock
released kernels.** Route B is declined; the question is settled, not
deferred. An app that wants sound ships a kernel fragment, builds a kernel
once (content-addressed and cached, so re-runs are instant), and builds with
`gosd build --artifacts-dir` — exactly the precedent DRM set with
`examples/sattrack`, and now `examples/chime` for audio.

### The size argument is conceded

This is worth recording precisely, because it was the argument Route B was
expected to win on, and it did: the measurement came out about **ten times
cheaper** than the DRM precedent suggested. A deliberate ALSA-core +
`snd_bcm2835` + deny-list configuration costs **+104,248 bytes (+0.63%)** on
the pi-zero-w's armv6 `kernel.img` — the most size-sensitive kernel GoSD ships,
where the kernel essentially *is* the artifact — and **+401,408 bytes (+0.71%)**
on the pi-zero-2w's arm64 `kernel8.img`. Both against the published
`artifacts/v0.8.0` kernels. For contrast, `examples/sattrack`'s DRM-driven
fragment costs +7.7%.

Two thirds of one percent is not a reason to say no to anything. If this
decision rested on size, Route B would have won on the Pi boards. It does not
rest on size, and the two reasons it turns on are ones size cannot touch:

1. **Rockchip HDMI audio requires DRM.** On RK3399/RK3566 the HDMI audio path
   is a codec hanging off the Synopsys DesignWare HDMI bridge
   (`DRM_DW_HDMI_I2S_AUDIO`), driven through ASoC. So a stock-kernel Route B
   either ships the whole DRM subsystem to every board — contradicting the
   decision that put DRM behind a recipe in the first place — or ships an
   "audio" feature that means HDMI on the Pis, analog-only on the ROCK 4SE,
   nothing usable on the radxa-zero-3e (HDMI-only hardware, so DRM or
   silence), and nothing at all on the nanopi-zero2 (no HDMI, no jack). A
   capability that varies that much per board is worse as a stock feature
   than as an opt-in recipe: the recipe at least tells you what you are
   getting.
2. **Any Pi kernel that gains sound wakes the dormant USB MIDI gadget.**
   `USB_MIDI_GADGET` and `USB_CONFIGFS_F_MIDI` depend on the raw-MIDI core,
   which cannot exist while `CONFIG_SOUND` is off — so they sit inert in every
   stock GoSD Pi kernel today (the gadget stack itself is already on) and wake
   the moment sound appears. Legacy gadget drivers claim the board's only UDC
   at probe: precisely how "Gadget Zero" broke `--usb-gadget` in bean
   gosd-spjt. Route B would therefore have to carry a deny-list into every Pi
   `kernel.fragment` and keep it correct across kernel bumps, or silently
   trade USB gadget mode for audio on the boards where gadget mode matters
   most. Measured cost of *not* denying them: +15,424 bytes of MIDI gadget
   nobody asked for, on top of the sound figures above.

Route A costs nothing to the apps that don't want audio, needs no artifacts
release, and is strictly additive and reversible. If a future case makes Route
B worth revisiting, the honest scope is still "Pi boards get `snd_bcm2835` in
stock kernels, Rockchip stays recipe-only", and gosd-y9hc's fragment is
exactly what gets promoted — a promotion, not a rewrite. Nothing shipped under
Route A has to be undone for that.

## Summary of Changes

Decision recorded, no code change. Route A confirmed by JP on 2026-08-21 with
the size argument explicitly conceded; the parent epic gosd-qkbl's "the fork —
JP to choose" is now resolved and noted there. What already shipped under
Route A stands as the answer to "can a GoSD app play sound": the public
`sound/` package (bean gosd-lrxz), `examples/chime` with its Pi custom-kernel
recipe (bean gosd-y9hc), the Rockchip recipes, and the audio documentation.
