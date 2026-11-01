---
# gosd-uw32
title: 'Jetson Nano: artifacts release + public activation'
status: todo
type: task
priority: normal
created_at: 2026-11-01T11:18:51Z
updated_at: 2026-11-01T20:01:23Z
parent: gosd-72je
blocked_by:
    - gosd-9o0m
---

Tag-first, bump-second, per CLAUDE.md, the same two-PR shape as gosd-wf58. PR 1 adds dist/jetson-nano.tar.zst to build-artifacts.yml's release upload list, with an artifacts: change file. After the release is published, PR 2 flips RegisterInternal to Register, bumps artifacts.Version via pin-bump.sh, and adds the fake-artifacts fixtures, the COMPATIBILITY.md row and columns, and end-user docs. Those docs must lead with the QSPI prerequisite: boot JetPack 4.5+ once.
