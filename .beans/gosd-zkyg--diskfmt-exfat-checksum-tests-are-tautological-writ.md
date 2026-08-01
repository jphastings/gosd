---
# gosd-zkyg
title: 'diskfmt: exFAT checksum tests are tautological — writer validated only against itself, a checksum bug would pass every test'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:59:13Z
updated_at: 2026-07-31T07:59:13Z
---

Found by review sweep `gosd-fuxs` (storage area), verified.

exfatformat_test.go:104-106 and :171 verify the boot-region and up-case
checksums by calling the production `exFATRollingChecksum` on both sides
of the assertion — f(x) == f(x). A wrong rotate direction, wrong excluded
offsets (spec excludes exactly 106/107/112), or wrong sector span would
pass every test while every real OS rejected the volume. The wider
exfat_test.go suite similarly validates the writer against GoSD's own
reader.

The reviewing agent closed the gap manually for this sweep: macOS
fsck_exfat reports the volume OK across the cluster-shift ladder (1MiB
minimum, ±512B around the 256MiB and 32GiB transitions, 1GiB) — **no live
defect exists today**; this bean is about making that fact mechanically
checkable.

**Fix:** pin the checksum with a hand-computed constant fixture (known
11-sector region + expected uint32 derived from the spec pseudo-code by
hand); optionally an opt-in CI job running fsck.exfat/fsck.vfat over
formatter output — the exact check that surfaced gosd-e3e3.
