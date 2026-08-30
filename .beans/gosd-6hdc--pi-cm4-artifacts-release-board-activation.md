---
# gosd-6hdc
title: 'Pi CM4: artifacts release + board activation'
status: todo
type: task
created_at: 2026-08-30T10:25:55Z
updated_at: 2026-08-30T10:25:55Z
parent: gosd-7676
---

## What

Once `pi-cm4`'s kernel is published in an `artifacts/vX.Y.Z` release,
flip `boards.RegisterInternal(picm4.New())` to `boards.Register(...)` in
`internal/boardset/boardset.go`, bump `internal/artifacts.Version` +
`ManifestSHA256`, add the COMPATIBILITY.md bring-up row + feature-table
column, and add `pi-cm4` to CLAUDE.md's "Board IDs" locked-decision list.

Same activation shape as `gosd-7wv9` (pi-3b) / `gosd-wf58` (turing-rk1).
Not urgent — this board has no real users yet; do this whenever the next
artifacts release happens to include it.
