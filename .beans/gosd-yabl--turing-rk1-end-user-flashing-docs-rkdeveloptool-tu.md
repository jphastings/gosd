---
# gosd-yabl
title: 'Turing RK1: end-user flashing docs (rkdeveloptool / Turing Pi 2 BMC, no Imager catalog)'
status: completed
type: task
priority: normal
created_at: 2026-08-25T10:26:48Z
updated_at: 2026-08-30T09:27:03Z
parent: gosd-bntd
blocked_by:
    - gosd-jvtg
---

This board has no SD slot, so the Raspberry Pi Imager custom-repository catalog flow (GoSD's flagship end-user path for every other board) doesn't apply -- Imager only drives card readers. Document the two working alternatives instead, both of which raw-write gosd's ordinary .img unmodified: (1) USB maskrom mode + rkdeveloptool from a host PC, (2) the Turing Pi 2 BMC web UI / tpi CLI upload. The config-tree hand-edit fallback (docs/config.md) is unaffected once the image is on eMMC -- note that explicitly so users don't think they've lost the always-present fallback. Likely a new docs/ page plus a COMPATIBILITY.md callout; check whether docs/provisioning-formats.md or the README's flashing-path section needs a pointer to it too.

## Summary of Changes

Shipped as docs/turing-rk1-flashing.md in PR #372 (the board-profile PR) --
covers rkdeveloptool (USB maskrom) and the Turing Pi 2 BMC/tpi, both
confirmed against Turing's own docs to raw-write gosd's ordinary .img
unmodified. Cross-linking from README.md/other entry points is deferred
until the board goes public (gosd-wf58) -- no point routing users to a
board they cannot yet build for.
