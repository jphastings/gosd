---
# gosd-df57
title: sattrack's kernel fragment silently compiles in the whole Pi audio zoo
status: in-progress
type: bug
priority: normal
created_at: 2026-07-29T21:45:24Z
updated_at: 2026-07-31T07:49:51Z
parent: gosd-qkbl
---

Its comment claims no codec/machine/USB drivers come with the SND re-enable. The built kernel.config says otherwise.

## Summary of Changes

Rewrote `examples/sattrack/kernel/pi-zero-w.fragment` (the only sattrack
fragment that touches sound - `qemu-virt.fragment` never enables
`CONFIG_SOUND`/`SND`, so it was not affected).

The three enable lines (`CONFIG_SOUND=y`, `CONFIG_SND=y`, `CONFIG_SND_SOC=y`)
are unchanged: `DRM_VC4` hard-depends on `SND && SND_SOC`
(`drivers/gpu/drm/vc4/Kconfig` at the pinned raspberrypi/linux commit
`63598c83153e19b1f99067ab6df7409de2c111f8`, verified by fetching the file),
and sattrack plays no audio of its own, so nothing beyond satisfying that
dependency is wanted.

Added a deny-list of ~70 real Kconfig symbols, each verified against the
actual `bcmrpi_defconfig` at the pinned commit (fetched from
raw.githubusercontent.com) and cross-checked spelling against the upstream
Kconfig files (`sound/core/Kconfig`, `sound/drivers/Kconfig`,
`sound/arm/Kconfig`, `sound/spi/Kconfig`, `sound/usb/Kconfig`,
`sound/soc/bcm/Kconfig`, `sound/soc/codecs/Kconfig`,
`sound/soc/generic/Kconfig`) so nothing invented slipped in:

- Generic ALSA menus with no hardware here: `SND_DRIVERS`, `SND_ARM`,
  `SND_SPI`, `SND_USB`.
- Named explicitly per the task requirement (also already covered by the
  `SND_DRIVERS` deny above, but called out by name given the real-world
  ALOOP incident in a sibling project): `SND_ALOOP`, `SND_DUMMY`.
- No MIDI/sequencer/OSS/legacy API: `SND_SEQUENCER`, `SND_OSSEMUL`,
  `SND_HRTIMER`, `SND_SUPPORT_OLD_API`.
- ~40 Pi HAT machine drivers (`SND_BCM2708_SOC_*`, HiFiBerry, IQaudio,
  JustBoom, Allo, AudioInjector, Pisound, DionAudio, etc.) and ~14 generic
  ASoC codecs they reference (`SND_SOC_AD193X_*`, `WM8904`, `WM8960`, etc.),
  denied individually by name - unlike examples/chime's `pi.fragment`,
  `# CONFIG_SND_SOC is not set` cannot do this work here because sattrack
  needs `SND_SOC=y` for vc4.
- The Pi's own audio driver, `SND_BCM2835` and its I2S companion
  `SND_BCM2835_SOC_I2S` - sattrack has no playback code at all, so even this
  is denied.
- `USB_MIDI_GADGET` and `USB_CONFIGFS_F_MIDI` (the raw-MIDI-core-wakes-the-
  legacy-USB-gadget trap from bean gosd-spjt/gosd-y9hc) - real symbols in
  this defconfig (`USB_CONFIGFS_F_MIDI=y`, `USB_MIDI_GADGET=m`), confirmed
  dormant only because `CONFIG_SOUND` was off.

Also fixed the fragment's comment, which previously made the false claim
this bug is about; it now explains the defconfig-promotion trap (why a
three-line "just satisfy the dependency" re-enable silently ships the whole
audio ecosystem in a monolithic kernel) and points at this bean.

**Artifact-release rule does not apply.** `examples/sattrack/kernel/` is an
opt-in custom-kernel recipe consumed via `gosd build-kernel`, not a file
under `build/boards/*` that feeds the stock `gosd build` pipeline - so
CLAUDE.md's "tag-first, bump-second" artifact-release process and
`internal/artifacts.Version` are not implicated. This PR needs no artifacts
release.

**Outstanding, deliberately not done here (per task constraint):** no
`gosd build-kernel --board pi-zero-w` run and no re-measurement of the
sattrack kernel size (the epic bean gosd-qkbl's sattrack row - "DRM + vc4 +
its minimal sound", 17,760,952 bytes / +7.7% - was measured against the
*old, over-permissive* fragment, so it is now a stale upper bound, not a
lower one; the corrected fragment should build smaller). Whoever verifies
this should run the build, diff `kernel.config` against the deny-list above
to confirm no denied symbol survived as `=y`, and update the epic's
"Measured size evidence" table if the sattrack row is worth keeping.
Leaving this bean `in-progress` rather than `done` until that verification
happens.
