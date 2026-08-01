---
# gosd-vcnr
title: 'wifiup: backoff resets on CONNECT-accepted, not on confirmed association — wrong PSK reconnect-storms every ~3s forever'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:53:06Z
updated_at: 2026-08-01T17:39:56Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

wifiup.go:217-243: `associate()` success → `backoff.Reset()` — but the
adjacent comment itself says the CONNECT ack only means the kernel
accepted the request; the 4-way handshake is still in flight.
`runUntilDisconnect` returns as soon as the 3s association poll sees
`Associated == false`, so the loop never grows its delay.

**Failure scenario:** wrong passphrase in gosd.toml — the single most
common misconfiguration: Disconnect → ConnectPSK (accepted) → Reset →
3s poll → not associated → repeat at a fixed ~3s cadence forever.
Continuous handshake attempts (some APs blacklist for this), serial log
spam, nl80211 churn; the backoff's stated purpose is defeated.

**Fix:** reset only once `Associated` has actually been observed true
(move Reset into watchAssociation or return it from runUntilDisconnect);
a cycle that never associated takes `backoff.Next()` like a failed
associate.

## Summary of Changes

- `cmd/gosd-init/internal/wifiup/wifiup.go`: `watchAssociation` now
  tracks whether `Associated` was ever observed true and returns that
  bool; `runUntilDisconnect` propagates it. `runAssociationLoop` no
  longer calls `backoff.Reset()` right after the CONNECT ack — it only
  resets once `runUntilDisconnect` reports a genuine (if since-lost)
  association. A cycle that never associated instead calls
  `backoff.Next()` and waits, exactly like a failed `associate()`, with
  its own log line ("accepted the connect but association was never
  confirmed"). A long-lived, genuinely-associated session that later
  drops still resets and reconnects promptly (unchanged).
- `cmd/gosd-init/internal/wifiup/fakes_test.go`: `fakeWifiClient` gained
  `connectErrs` (a per-call scripted Connect outcome sequence, mirroring
  the existing `interfacesResults`/`associatedResults` polling
  convention) so a test can mix failed and successful connects across
  cycles; `testLog` gained `count(substr)` for waiting on a specific
  occurrence of a repeated log line.
- `cmd/gosd-init/internal/wifiup/wifiup_test.go`: three new behavioral
  tests — `TestRunNeverAssociatingBacksOffAcrossRepeatedCycles` (a
  wrong-PSK-style connection that never associates is gated by a
  backoff wait on every cycle, not the old fixed ~3s poll cadence),
  `TestRunDoesNotResetBackoffOnImmediateDisassociation` (the minimal
  single-cycle case), and `TestRunResetsBackoffAfterGenuineAssociation`
  (a genuine association-then-loss resets the backoff even after prior
  cycles had grown it — proven deterministically via
  `Backoff.Next`'s full-jitter draw always being strictly less than its
  base delay on the first call after a reset).

## Todos

- [x] Move the backoff reset so it fires only on confirmed association,
      not on the CONNECT ack
- [x] Preserve prompt reconnect for a genuinely-associated session that
      later drops
- [x] Behavioral tests: never-associates growth, reset-on-genuine-
      association, ack-then-immediate-disassociation does not reset
- [x] Quality gates (go test/vet, gofmt, golangci-lint darwin + linux)
