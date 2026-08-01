---
# gosd-0esw
title: 'timesync: no maximum-step or sanity-floor on SNTP results — one spoofed UDP response sets the clock to any value'
status: completed
type: task
priority: normal
tags:
    - security
created_at: 2026-07-31T07:53:06Z
updated_at: 2026-08-01T18:05:42Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`applySync` (cmd/gosd-init/internal/timesync/timesync.go:194-201) applies
`newTime` unconditionally; beevik/ntp's `Validate()` checks only
stratum/leap/dispersion/freshness — all attacker-controlled in a forged
packet. Resync repeats hourly over unauthenticated UDP for the life of the
device.

**Failure scenario:** an on-path/LAN attacker races a forged response;
settimeofday lands 2019 or 2099. `/run/gosd/time-synced` already exists,
so apps gating TLS on it now validate certificates against a bogus clock —
accepting expired certs or failing closed — with no RTC, no interactive
surface, and no remote way to notice or fix it.

**Fix:** refuse any time before a build-baked floor (the build date is
known at image build); after first sync, cap the step (ntpd's classic
~1000s panic threshold) and log-and-refuse larger; optionally require
agreement from 2+ servers for the first step.



## Summary of Changes

- **Floor**: `gosd build` now bakes `config.json.buildTimestamp` (UTC,
  RFC3339Nano) at image-assembly time (`internal/pipeline.Assemble`).
  `initcfg.Config.BuildTime()` parses it back, treating empty/malformed as
  the zero time.Time ("no floor available", not "before the epoch") —
  mirroring `ShortIdentity`'s handling of a missing `Identity`.
  `config.json` is excluded from `ComputeIdentity`'s hashed payload in its
  entirety (see that function's docstring), so a value that necessarily
  differs on every build never touches build reproducibility;
  `TestBuildTimestampVariesButIdentityDoesNotAcrossRebuilds`
  (cmd/gosd/build_integration_test.go) proves both halves of that claim in
  one test. `cmd/gosd-init/main.go` wires `cfg.BuildTime()` into
  `timesync.Options.Floor`; `timesync.checkFloor` refuses (and logs) any
  SNTP result before it, for both the first sync and every resync.
- **Step guard**: `timesync.Options.MaxStep` (default `DefaultMaxStep` =
  1000s, ntpd's classic panic threshold) bounds how far a *resync* (never
  the first sync, which has no trustworthy baseline) may step the clock
  outright. The new `stepGuard` (cmd/gosd-init/internal/timesync/guard.go)
  tracks the mapping between the scheduling `Clock`'s timeline and the
  system clock's last-applied value itself, rather than reading the system
  clock back (`SystemClock` deliberately only exposes `Set`) — this keeps
  the guard pure and fake-testable without wiring the test fakes together.
  A candidate beyond `MaxStep` is refused and logged, remembered as
  "pending"; it's only applied once the *immediately following* resync
  reports a candidate whose movement matches the real elapsed time between
  the two queries (i.e. the implied offset stayed ~constant, exactly what a
  long-powered-off-but-still-ticking device's clock would report) — the
  bean's "requiring agreement from a second query" suggestion. A
  disagreeing second over-threshold reading is refused again and becomes
  the new pending candidate, rather than being treated as a confirmation.
- Behavioral tests added in `cmd/gosd-init/internal/timesync/timesync_test.go`:
  pre-floor result refused + logged + retried (marker withheld until a
  valid result lands); an ordinary in-threshold resync applied immediately;
  an over-threshold step refused once then applied once a second, agreeing
  query lands; a second over-threshold reading that disagrees stays
  refused. Existing tests (marker-once-on-first-sync, resync semantics,
  etc.) pass unmodified against the new `Options.Floor`/`Options.MaxStep`
  fields defaulting to disabled (zero value).
- Fixed a pre-existing TOCTOU race in several timesync tests (waiting only
  on `len(sys.sets())` before immediately asserting on `marked.load()`,
  which is set a statement later in the same goroutine) — surfaced by the
  larger number of goroutine-spawning tests in this package under `-race`;
  not a production bug, but was making both old and new tests flaky.
- `docs/runtime.md`'s SNTP section gained a paragraph on the floor/step
  guard behavior; `internal/initcfg/config_test.go` gained
  `TestConfigBuildTime` coverage; `internal/pipeline/pipeline_test.go`
  gained `TestAssembleBakesBuildTimestampIntoConfigJSON`.
- Verified the qemu boot-to-HTTP CI job stays green regardless of NTP
  reachability: `timesync.Run` is launched in its own guarded goroutine
  (`boot.PanicGuard`) and never blocks `/app`'s start either way, and
  neither guard's log lines are asserted on by that job's HTTP-polling
  script.

Gates run: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty),
`golangci-lint run ./...`, `GOOS=linux golangci-lint run ./...` — all
clean.
