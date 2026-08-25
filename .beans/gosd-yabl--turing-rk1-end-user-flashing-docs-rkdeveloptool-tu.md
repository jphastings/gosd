---
# gosd-yabl
title: 'Turing RK1: end-user flashing docs (rkdeveloptool / Turing Pi 2 BMC, no Imager catalog)'
status: todo
type: task
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-25T10:26:48Z
parent: gosd-bntd
blocked_by:
    - gosd-jvtg
---

This board has no SD slot, so the Raspberry Pi Imager custom-repository catalog flow (GoSD's flagship end-user path for every other board) doesn't apply -- Imager only drives card readers. Document the two working alternatives instead, both of which raw-write gosd's ordinary .img unmodified: (1) USB maskrom mode + rkdeveloptool from a host PC, (2) the Turing Pi 2 BMC web UI / tpi CLI upload. The config-tree hand-edit fallback (docs/config.md) is unaffected once the image is on eMMC -- note that explicitly so users don't think they've lost the always-present fallback. Likely a new docs/ page plus a COMPATIBILITY.md callout; check whether docs/provisioning-formats.md or the README's flashing-path section needs a pointer to it too.
