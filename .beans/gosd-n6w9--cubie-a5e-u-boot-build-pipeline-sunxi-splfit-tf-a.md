---
# gosd-n6w9
title: 'Cubie A5E: U-Boot build pipeline (sunxi SPL+FIT, TF-A from source)'
status: todo
type: task
priority: normal
created_at: 2026-08-06T22:33:44Z
updated_at: 2026-08-06T22:42:43Z
parent: gosd-h1wv
blocked_by:
    - gosd-jpc8
---

build/boards/cubie-a5e/{manifest.json,uboot/} mirroring rock-4se's blob-free pattern: Dockerfile + build.sh building mainline U-Boot radxa-a5e_defconfig with BL31 compiled from mainline TF-A (make PLAT=sun55i_a523), pins read from manifest.json, tag-verified against the peeled commit. Output artifact is the single u-boot-sunxi-with-spl.bin (docker cp out of a scratch stage).

Confirmed pins (gosd-jpc8): U-Boot mainline v2026.04, defconfig radxa-cubie-a5e_defconfig (board DT in-tree at that tag). TF-A from jernejsk/arm-trusted-firmware branch a523 @ b5de74a685fb73b784e45bbbd18dd9a0c528d8b2 — make PLAT=sun55i_a523 bl31, passed via BL31=; SCP=/dev/null (A523 uses no SCP). Keep the bootdelay0.config merge (same boot-time rationale as the other boards). Output artifact: u-boot-sunxi-with-spl.bin (binman FIT: SPL + BL31 + U-Boot proper + DTB), flashed at 8KiB.

## Todos

- [ ] manifest.json with tfa section (repo/tag/peeled commit/license note), schema note explaining the sunxi blob-free chain
- [ ] uboot/Dockerfile (TF-A bl31 stage + U-Boot stage + scratch artifacts stage) and build.sh (jq-driven pins, out/ copy)
- [ ] uboot/README.md: boot-chain explanation, offset, how to bump pins
- [ ] Local Docker build succeeds; record u-boot-sunxi-with-spl.bin size (must fit the pre-partition gap with room to spare)
- [ ] Quality gates + PR
