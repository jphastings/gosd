---
# gosd-cjr6
title: 'cubie-a5e: repin TF-A to mainline when sun55i_a523 lands upstream'
status: todo
type: task
priority: low
created_at: 2026-08-07T19:04:27Z
updated_at: 2026-08-07T19:04:27Z
parent: gosd-h1wv
---

build/boards/cubie-a5e/manifest.json pins BL31 from jernejsk/arm-trusted-firmware branch a523 @ b5de74a685fb (commit-authoritative) because mainline TF-A has no sun55i_a523 platform at any release tag (verified 2026-08-06, bean gosd-jpc8). When mainline TF-A ships the platform (watch plat/allwinner/ in releases), repin to the mainline tag: manifest tfa section, Dockerfile fetch path, README wording — then the usual artifacts release dance. Until then the fork pin is deliberate and fine (source-compiled, BSD-3-Clause; precedent: the Pi boards' raspberrypi/linux commit pin).
