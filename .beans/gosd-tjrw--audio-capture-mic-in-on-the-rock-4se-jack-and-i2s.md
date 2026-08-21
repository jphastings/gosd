---
# gosd-tjrw
title: 'Audio capture: mic in on the ROCK 4SE jack, and I2S mics'
status: scrapped
type: task
priority: deferred
created_at: 2026-07-29T21:45:25Z
updated_at: 2026-08-21T04:42:06Z
parent: gosd-qkbl
---

Deferred from gosd-qkbl: same ioctl path with READI_FRAMES on /dev/snd/pcmC*D*c.


## Reasons for Scrapping

**JP, 2026-08-21: closed as won't-do.**

Capture — mic in on the ROCK 4SE's 4-ring jack, and I2S mics off the headers —
has no consumer. No bean wants it, no example wants it, and no app has asked
for it. It was filed as an "if it turns out to be needed" deferral out of
gosd-qkbl's research, and it did not turn out that way.

Nothing about it got harder by waiting, which is the other half of why it can
close cleanly: it is the same ALSA PCM ioctl path the `sound/` package already
speaks, with `SNDRV_PCM_IOCTL_READI_FRAMES` against
`/dev/snd/pcmC*D*c` instead of `WRITEI` against the playback node, and the
same per-arch struct-size discipline (`uintptr` fields, ioctl size derived
from `unsafe.Sizeof`) that package already pins with tests. The kernel side
is the same opt-in `gosd build-kernel` recipe that Route A settled for
playback on 2026-08-21 (bean gosd-ette) — the 4SE's ES8316 codec handles both
directions, so a recipe that gets analog out also gets analog in.

**Re-file it the day something needs a microphone.** The work is small,
well-understood and unblocked; what it lacks is a reason.
