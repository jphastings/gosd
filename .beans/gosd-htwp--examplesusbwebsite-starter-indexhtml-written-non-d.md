---
# gosd-htwp
title: 'examples/usbwebsite: starter index.html written non-durably and never repaired if truncated'
status: todo
type: bug
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-07-31T07:54:53Z
---

Found by review sweep `gosd-fuxs` (cross-cutting area), verified.

ensureStarterPage (examples/usbwebsite/main.go:364-381) does a bare
os.WriteFile to the FAT volume, guarded by an existence check — so a
power cut mid-write leaves a truncated/empty index.html that the
existence check then treats as user content and never rewrites. Every
other write in the examples follows docs/runtime.md's "Making a write
durable" sequence (hello and emmcstorage both carry writeFileDurably).

**Fix:** reuse the durable-write sequence for the starter page (worth
factoring the helper into a small shared spot for examples, or accept the
duplication as the examples' stdlib-only style does elsewhere).
