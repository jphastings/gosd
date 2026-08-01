---
# gosd-4k5k
title: 'cmd/gosd: --data-size validates the 256GiB ceiling up front but not the sub-sector floor — a full build runs before failing'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:53Z
updated_at: 2026-07-31T07:54:53Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

parseDataSize (cmd/gosd/build.go:282-310) rejects negatives, overflow, and
> MaxFAT32Bytes — but 0 < size < 512 passes and only fails deep in
computeLayout (image.go:121-124) after cross-compiling the app and
gosd-init and fetching artifacts for every board. The ceiling check's own
doc says it exists to refuse "before any image bytes exist".

**Fix:** symmetric floor check in parseDataSize (reject 0 < size < 512
with the same actionable style). Arguably reject anything below a useful
minimum (one FAT32 cluster + overhead) with a message naming it.
