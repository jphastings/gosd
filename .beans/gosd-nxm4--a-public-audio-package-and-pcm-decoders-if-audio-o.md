---
# gosd-nxm4
title: A public audio/ package, and PCM decoders, if audio outgrows one example
status: todo
type: task
priority: deferred
created_at: 2026-07-29T21:45:25Z
updated_at: 2026-07-29T21:45:25Z
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
