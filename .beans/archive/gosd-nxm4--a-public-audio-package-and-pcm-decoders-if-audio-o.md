---
# gosd-nxm4
title: A public audio/ package, and PCM decoders, if audio outgrows one example
status: scrapped
type: task
priority: deferred
created_at: 2026-07-29T21:45:25Z
updated_at: 2026-08-21T04:42:06Z
parent: gosd-qkbl
---

Deferred from gosd-qkbl: the example's ALSA PCM ioctl code could become public API; decoding (MP3/Vorbis/FLAC) is a separate third-party-dependency question.

**Half of this shipped in gosd-lrxz.** The public package landed as `sound/`
(not `audio/`), because playback is what it does: `Open`/`OpenWith`, a `Device`
you `Play` interleaved S16_LE frames to, `ErrNoDevice` with an actionable
message. It is semver-relevant public API alongside `gadget/`, `emmc/` and
`disk/`. `examples/chime` is a consumer of it rather than a copy source.

What remains here, all deliberately left out of that package:

- **Decoders.** WAV/raw PCM is an `encoding/binary` header parse, so it may not
  even want a decoder API; MP3/Vorbis/FLAC mean third-party dependencies
  (`hajimehoshi/go-mp3` is pure Go but **archived** in 2023,
  `jfreymuth/oggvorbis`, `mewkiz/flac`) and the same judgement call the disk
  work made about `yobert/alsa`.
- **Device enumeration** — a `sound.Devices()` mirroring `disk.Devices()`, for
  an app that wants to choose or log every PCM rather than take the ranked
  first one. `Options{Path, Prefer}` covers today's needs.
- **Mixing, capture and non-blocking playback.** Capture is `gosd-tjrw`;
  mixing two streams means an app-level mixer or ALSA's `dmix` plugin, which
  does not exist without alsa-lib.


## Reasons for Scrapping

**JP, 2026-08-21: closed as won't-do.**

This bean's real deliverable already shipped. The public package landed as
`sound/` via bean gosd-lrxz — `Open`/`OpenWith`, a `Device` you `Play`
interleaved S16_LE frames to, `ErrNoDevice` with an actionable message — and
it is semver-relevant public API alongside `gadget/`, `emmc/` and `disk/`.
`examples/chime` consumes it rather than copying from it. The "if audio
outgrows one example" condition in the title was met, and answered, months
ago.

What was left here was **MP3/Vorbis/FLAC decoding**, plus two smaller
maybes (`sound.Devices()` enumeration, mixing). Decoding is a third-party-
dependency question — `hajimehoshi/go-mp3` is pure Go but archived in 2023,
`jfreymuth/oggvorbis`, `mewkiz/flac` — of exactly the kind the disk work
turned down over `yobert/alsa`, and **no bean, example or app is asking for
it**. WAV/raw PCM, which is what an appliance playing a chime or a prompt
actually needs, is an `encoding/binary` header parse the app can do in a
dozen lines with no API from us at all.

This was an "if it turns out to be needed" bean, and it did not turn out that
way. **Re-file it the day something needs a decoder** — the shape of the work
is unchanged and this bean's body records the candidate libraries and their
maintenance status, so re-filing costs nothing.
