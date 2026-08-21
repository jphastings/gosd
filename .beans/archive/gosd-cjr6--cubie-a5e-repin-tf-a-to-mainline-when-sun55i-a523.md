---
# gosd-cjr6
title: 'cubie-a5e: repin TF-A to mainline when sun55i_a523 lands upstream'
status: scrapped
type: task
priority: low
created_at: 2026-08-07T19:04:27Z
updated_at: 2026-08-21T04:43:59Z
parent: gosd-h1wv
---

build/boards/cubie-a5e/manifest.json pins BL31 from jernejsk/arm-trusted-firmware branch a523 @ b5de74a685fb (commit-authoritative) because mainline TF-A has no sun55i_a523 platform at any release tag (verified 2026-08-06, bean gosd-jpc8). When mainline TF-A ships the platform (watch plat/allwinner/ in releases), repin to the mainline tag: manifest tfa section, Dockerfile fetch path, README wording — then the usual artifacts release dance. Until then the fork pin is deliberate and fine (source-compiled, BSD-3-Clause; precedent: the Pi boards' raspberrypi/linux commit pin).


## Reasons for Scrapping

**JP, 2026-08-21: superseded by gosd-woox, the single upstream watch list.**
Not abandoned — re-homed. This is the one of the four whose trigger is *not*
the fleet kernel tag bump, and it moves anyway: it is the same kind of thing
(a pin held on a fork until upstream catches up, on a board whose other
deferrals are already on that list), and splitting one upstream-watch item off
onto its own bean is how it gets forgotten.

gosd-woox's item W4 carries it exactly: watch `plat/allwinner/` in TF-A
releases for a `sun55i_a523` platform (trigger T2, independent of the kernel
tag); until then the `jernejsk/arm-trusted-firmware` `a523` pin at
`b5de74a685fb` is **deliberate and fine** — source-compiled, BSD-3-Clause,
precedented by the Pi boards' raspberrypi/linux commit pin, and not technical
debt to pay down early. When mainline ships it: repin the manifest's `tfa`
section, the Dockerfile fetch path and the README wording, then the usual
artifacts release dance.
