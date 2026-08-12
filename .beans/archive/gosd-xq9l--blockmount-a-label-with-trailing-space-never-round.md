---
# gosd-xq9l
title: 'blockmount: a label with trailing space never round-trips — destructive=true reformats the app''s own data every boot'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:58:28Z
updated_at: 2026-07-31T07:58:28Z
---

Found by review sweep `gosd-fuxs` (storage area), empirically verified on
both filesystems (format with "APPDATA " → Inspect reads "APPDATA").

`ValidateLabel` (blockmount.go:162-175) accepts any 1-11 printable-ASCII
label including trailing spaces, but both readers strip trailing
padding (`trimLabel` TrimRight " \x00"; decodeExFATLabel same), and Run's
idempotency check compares `strings.EqualFold(contents.Label, label)`
against the UNTRIMMED caller string (blockmount.go:103).

**Failure scenario:** `disk.FormatAndMount("APPDATA ", "/storage", true)`
(trailing space from config/env/Join). Boot 1: formats, mounts, works.
Every subsequent boot: read-back label ≠ requested label → default branch
→ reformat → the app's entire persistent dataset destroyed, silently, with
a success result. With destructive=false: permanent ErrRefusedFormat the
app can never escape.

**Fix:** reject leading/trailing space and NUL in ValidateLabel (such a
label provably cannot round-trip), and belt-and-braces compare against
`trimLabel(label)` in Run. Behavioral test: label round-trip through
format→Inspect for every label class ValidateLabel admits.

## Summary of Changes

- `internal/blockmount.ValidateLabel` now refuses a label with a leading or
  trailing space (and the all-spaces case) with an actionable error naming
  the problem and, where there's real content left, suggesting the trimmed
  label as the fix. NUL was already refused by the existing printable-ASCII
  check (confirmed with a dedicated test, not duplicated).
- Belt-and-braces: `Run`'s idempotency comparison is now the small
  `labelMatches` helper, which compares `contents.Label` against
  `trimLabel(label)` (a local mirror of `diskfmt`'s edge-padding trim,
  applied to both edges) rather than the raw caller string — so even a
  future validation bypass can't provoke the reformat-every-boot loop this
  bean describes.
- Tests: `ValidateLabel` edge-space rejection (with actionable-message and
  NUL-non-duplication checks), `labelMatches` trimmed-comparison behaviour,
  and — the round-trip test this bean specifically asks for —
  `TestAdmittedLabelsRoundTripToWhatRunCompares`, which really formats and
  Inspects a representative set of ValidateLabel-admitted labels on both
  FAT32 and exFAT and asserts `labelMatches` would recognise each result as
  already provisioned. `emmc`/`disk` each got a trailing-space case added to
  their existing `TestLabelErrorsAreAttributedToThisPackage`, confirming the
  new error surfaces through both public packages with their own prefix.
- **Finding beyond this bean's scope, filed separately as `gosd-f83b`:** the
  round-trip testing done here shows this bean's "interior spaces round-trip
  (they do)" premise is not quite universal — a space landing exactly at
  byte 8 of a FAT32 label (e.g. `"ABCDEFG H"`) is silently dropped on
  read-back, for an unrelated reason (go-diskfs's 8.3 short-name/extension
  split trims each sub-field independently). This is the same failure class
  and consequence as this bean, just a narrower trigger untouched by the
  edge-space fix here; exFAT is unaffected. Filed as its own bug rather than
  folded into this PR, to keep this one scoped to the edge-space fix it was
  written for.
