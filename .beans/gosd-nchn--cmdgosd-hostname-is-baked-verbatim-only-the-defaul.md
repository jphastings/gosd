---
# gosd-nchn
title: 'cmd/gosd: --hostname is baked verbatim — only the default path is sanitized'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:54:11Z
updated_at: 2026-07-31T07:54:11Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

`--hostname` help text says "(default: sanitized main package name)" but
only the default runs through `naming.Sanitize` (cmd/gosd/build.go:162-165,
run.go:90-93); an explicit value is baked verbatim. Every comparable flag
validates at parse time (--env regex, --data-size, --console-baud).

**Failure scenario:** `--hostname "My Device!"` builds silently; mDNS
resolution of the invalid DNS label breaks (or sethostname rejects it) —
discovered on the bench, not at the flag. Also: `naming.Sanitize` has no
length cap, so even the default path can exceed sethostname's 64-byte
limit for a long package name.

**Fix:** validate/sanitize explicit --hostname at parse time with an
actionable error (or sanitize + log what changed), and add a 63-byte cap
to naming.Sanitize. Companion runtime bean (gosd-jeaw) hardens the
device side; do both.
