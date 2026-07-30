---
# gosd-cfkd
title: 'sound: nothing is audible until something unmutes the card (ALSA control support)'
status: completed
type: bug
priority: normal
created_at: 2026-07-30T16:42:25Z
updated_at: 2026-07-30T16:42:30Z
parent: gosd-qkbl
---



## Bench verification (2026-07-30) — HEARD, on the ROCK 4SE

A tone from `examples/chime` was audible from the board's 3.5 mm jack, on the
analog recipe. This is the first sound GoSD has ever produced, and it confirms
both halves of this bean: the diagnosis (the card was fine; the codec was
silent) and the fix (the audibility pass).

What the pass changed, from the boot's own control dump — all four were
holding the output down, in two different ways:

    numid=2  "Headphone Mixer Volume":                0,0 -> 8,8    (level, >=75% of 0..11)
    numid=4  "DAC Playback Volume":                   0,0 -> 144,144 (level, >=75% of 0..192)
    numid=33 "Left Headphone Mixer Left DAC Switch":  0 -> 1        (unmute the playback path)
    numid=35 "Right Headphone Mixer Right DAC Switch": 0 -> 1       (unmute the playback path)

Two volumes at zero *and* the DAPM route switches off, so raising the volumes
alone would still have produced silence — which is why the heuristic has to
understand switches, levels and enums rather than just "turn it up".

Evidence the conservative matching works as intended, from the same dump:
`Differential Mux` (the line-input selector, `lin1-rin1`) was left untouched,
and `Headphone Playback Volume` was already at its 0..3 maximum so it was not
rewritten — the pass only raises what is low and never reaches into the
capture path.

Still unheard: the ROCK 4SE's HDMI variant, and every Pi board (their recipes
compile and their configs check out, but nothing has been played through one).
