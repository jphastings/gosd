---
# gosd-6i2a
title: 'dataexpand: transient read failure on an established data partition is treated as corruption and halts the device'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:53:30Z
updated_at: 2026-08-01T18:01:58Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`verifyEstablished` (cmd/gosd-init/internal/dataexpand/dataexpand.go:193-206)
wraps both "device node missing" (checked once, no retry) and "Inspect
read failed" in `ErrDataCorrupt`, which boot.Run routes to
`haltForDataCorruption` → Halt. The creation path polls the node for 5s
(`waitForNode`) precisely because there is no udev to synchronize on; the
established path checks exactly once. This is the only place in gosd-init
where a transient I/O hiccup escalates to a terminal halt — everything
else retries.

**Failure scenario:** an intermittent EIO/EBUSY on an otherwise healthy
card → boot-failure.log tells the owner to reformat partition 2 → device
halts (not reboots). A retry would have succeeded; instead the device
needs a physical visit and the log actively advises destroying good data.

**Fix:** reuse waitForNode's poll shape for both checks; a persistent
*read* failure returns a non-ErrDataCorrupt error so boot falls through to
the read-only /data placeholder. Reserve ErrDataCorrupt + halt for a
successful read that is definitively not a GOSD-DATA FAT32 volume.

## Summary of Changes

Re-verified against current `main` (post gosd-lirl): `verifyEstablished`
still checked `PathExists` once and wrapped any `Inspect` error in
`ErrDataCorrupt`, exactly as filed — the bug had moved (line numbers, MBR-
derived offsets, re-adoption) but not closed.

- Added `nodeAppears` (waitForNode's poll loop, factored out so it has no
  caller-specific error text) and `pollInspect` (same poll shape, retrying
  `Inspect` instead of `PathExists`). `verifyEstablished` now retries both
  the device-node check and `Inspect` for `opts.NodeTimeout` before giving
  up, and only wraps `ErrDataCorrupt` around a *successful* read that shows
  the wrong contents. A persistent failure to even read (missing node or
  repeated `Inspect` error) returns a plain error, so `boot.Run`'s existing
  `errors.Is(err, ErrDataCorrupt)` gate falls through to the read-only
  `/data` placeholder and retries next boot, instead of halting.
- Cross-referenced this against `survivorPresent` (the CREATE-path
  equivalent, which already treats a failed `Inspect` as abort-not-format):
  doc comments on both functions now point at each other and spell out
  where the two paths deliberately diverge (debris-reformat vs.
  ErrDataCorrupt) once a read *succeeds*.
- Tests: a transient `Inspect` failure (fails 3x then succeeds) verifies
  clean with no halt; a persistent `Inspect` failure and a device node that
  never appears both now return non-`ErrDataCorrupt` errors instead of
  halting (replacing the old test that asserted the opposite); the existing
  successful-read-of-wrong-contents cases still assert `ErrDataCorrupt`.
  `boot` package already has generic coverage of both the halt and
  continue-read-only outcomes of `ExpandData`'s error (untouched).

Gates all green: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty),
`golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...`.
