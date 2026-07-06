---
# gosd-bo3z
title: Fix dangling rpi-imager permalinks in provisioning-formats.md
status: todo
type: task
priority: low
created_at: 2026-07-06T00:34:45Z
updated_at: 2026-07-06T00:34:45Z
---

PR #28 discovered the pinned rpi-imager commit 204a6eee... cited throughout docs/provisioning-formats.md no longer resolves on GitHub (likely a rebased/deleted ref). Tag v2.0.10 resolves to commit 467be3d3... with byte-identical content for the files we checked. Rewrite the ~30 permalinks to the v2.0.10 tag commit, spot-checking each cited line number still matches (line numbers can shift between the dangling commit and the tag). internal/catalog/testdata/README.md is already fixed — this bean is only the research doc.
