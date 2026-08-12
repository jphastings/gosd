---
# gosd-zkyg
title: 'diskfmt: exFAT checksum tests are tautological — writer validated only against itself, a checksum bug would pass every test'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:59:13Z
updated_at: 2026-08-07T17:07:53Z
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



## Todos

- [x] Pin `exFATRollingChecksum` against values worked out by hand from the spec pseudocode (rotate direction + accumulation), with and without the boot region's exclusion pattern
- [x] Add `specRollingChecksum`, a second implementation transcribed independently from the spec pseudocode (spelled out with shifts, not `bits.RotateLeft32`), to serve as a non-tautological oracle
- [x] Rewire `TestFormatExFATBootRegionValidates` to check the writer's on-disk checksum against the independent oracle instead of the production function it is meant to be validating
- [x] Audit the up-case table self-check for the same tautology and fix it the same way (`TestFormatExFATUpcaseTableIsWhatItClaims`)
- [x] Adversarially verify the fix: flip the production rotate direction, confirm all three checksum tests fail; revert
- [x] Quality gates + PR



## Summary of Changes

- `internal/diskfmt/exfatformat_test.go`: added `specRollingChecksum`, a second implementation of the exFAT spec's rolling checksum transcribed independently from the published pseudocode (`((checksum << 31) | (checksum >> 1)) + data[i]`, spelled out with shifts rather than `bits.RotateLeft32`), plus `TestExFATRollingChecksumMatchesTheSpecByHand` pinning both the production and independent implementations against a value worked out by hand for a 4-byte input, with and without the boot region's skip pattern (derivation shown step by step in the test's doc comment so a reviewer can redo it). `TestFormatExFATBootRegionValidates` and `TestFormatExFATUpcaseTableIsWhatItClaims` now compare the writer's on-disk checksums against `specRollingChecksum` instead of the production `exFATRollingChecksum`, closing the f(x)==f(x) tautology the bean found in both self-checks — a rotate-direction or excluded-offset bug now fails all three tests instead of none.
- Verified adversarially: temporarily flipped the production rotate (`bits.RotateLeft32(sum, -1)` to `..., 1)`), confirmed the new hand-pinned test and both rewired self-checks all fail, then reverted before committing.
- No production code changed — `exfatformat.go`'s checksum was already correct (the bean records macOS fsck_exfat validating real output across the cluster-size ladder); this closes the gap in how that correctness is mechanically checked.
- Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...` all green.
- PR stacked on #201 (gosd-8rw2).
