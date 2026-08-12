---
# gosd-dqps
title: 'timesync: RTC-less boot must converge via NTP alone — floor silently inactive + bogus first sync causes 56-year step refusals'
status: completed
type: bug
priority: high
created_at: 2026-08-07T13:04:22Z
updated_at: 2026-08-07T16:06:13Z
---

JP field report (2026-08-07, serial console, RTC-less device):

    [00:09:02.069] [gosd] NTP resync wants to step the clock by
    496139h32m6.88841185s, which exceeds the 16m40s max-step threshold;
    refusing until a second query agrees

496139h ≈ 56.6 years = epoch→2026. Requirement (JP): devices without an RTC
must be able to manage time exclusively through NTP on boot, robustly.

## Evidence chain (from code reading, this session)

1. The max-step message only fires in stepGuard.check, only on resyncs, and
   only AFTER checkFloor passed — so the refused candidate cleared the floor.
2. step ≈ 56.6y with a plausibly-correct 2026 candidate ⇒ guard's expected()
   ≈ epoch ⇒ the FIRST sync applied an epoch-ish time.
3. timesync's realClock.Now() is plain time.Now(): monotonic-carrying, so
   anchorClock Sub math measures true elapsed across the Set jump — the
   anchor arithmetic is NOT the bug.
4. ⇒ two co-occurring failures: (a) the build-timestamp floor was INACTIVE on
   this device (zero Floor disables checkFloor — config.json missing/
   unparseable buildTimestamp, per initcfg.BuildTime's "no floor available"),
   AND (b) the first sync's NTP response was bogus epoch-ish garbage — the
   classic source being an UNSYNCHRONIZED server (fresh-booted router: LI=3,
   stratum 0, era-zero timestamps), which queryServers currently does not
   validate at all.
5. Self-heal today: the NEXT resync's candidate agrees (offset constant,
   clocks tick at the same rate) and is applied — but the device sits at 1970
   for up to 2×ResyncEvery, TLS broken the whole time, with only this one
   cryptic line.

## Fix plan (todos)

[ ] Investigate the field device: does its /etc/gosd/config.json carry
    buildTimestamp? Which NTP server answered the first sync? Record here.
    (Deferred: this agent worked in an isolated worktree with no access to the
    field device — needs a bench/serial session, e.g. via the sdwire skill.)
[x] SNTP response validation in queryServers: reject LI=3 (alarm/unsync),
    stratum 0 or >15, zero transmit timestamp. An unsynced home router is
    the expected bench case, not an exotic attack. Implemented in validateSample
    (cmd/gosd-init/internal/timesync/timesync.go), run by queryServers on every
    response, real or faked — see sntp_test.go.
[x] Floor must never be silently absent: gosd build fails (or loudly warns)
    if it would bake an empty buildTimestamp; gosd-init logs ONE boot line
    when the floor is disabled so this state is visible in the field. gosd-init
    half: done, one line in timesync.Run (not per-attempt) — see
    TestRunLogsOnceWhenFloorIsDisabled. Build half: VERIFIED, not edited (owned
    by another in-flight stack; out of this bean's scope) — internal/pipeline's
    config.json marshal sets BuildTimestamp from time.Now().UTC() unconditionally,
    with no code path that can leave it empty, so "gosd build fails/warns on an
    empty buildTimestamp" is currently moot: a config.json missing it can only
    come from an image built by a gosd release that predates the field, not from
    today's pipeline. Checking this off on that basis.
[x] Fast recovery from a provably-bogus clock: when expected() < Floor, a
    forward step to ≥ Floor needs NO two-query agreement — the clock cannot
    legitimately read pre-build. (Keeps full two-query protection for all
    plausible-clock cases; gosd-0esw's threat model intact.) Implemented in
    stepGuard.check (guard.go); TestStepGuardFastPathsForwardStepFromPreFloorAnchor
    and TestStepGuardKeepsTwoQueryProtectionForPlausibleClock cover the fast path
    and gosd-0esw's still-intact protection respectively.
[x] Refusal log gains expected/floor context so field lines like JP's are
    self-diagnosing. Done: the max-step refusal line now names the candidate,
    expected(), and the floor ("none" when disabled) — see
    TestStepGuardRefusalLogIncludesExpectedAndFloor.
[x] Pending-confirmation resyncs shouldn't wait a full ResyncEvery: schedule
    the confirming query sooner (e.g. 30-60s) to bound wrong-clock time. Added
    Options.PendingConfirmDelay (default 45s, DefaultPendingConfirmDelay) and
    Run now uses it instead of ResyncEvery whenever a stepGuard confirmation is
    outstanding. Zero falls back to the default internally (unlike
    ResyncEvery/MaxStep, a zero delay has no useful meaning here), so main.go's
    unwired Options literal already gets the shorter delay with no change
    needed there — see pendingConfirmDelay and
    TestRunSchedulesConfirmingResyncSoonerThanResyncEvery.
[x] Test-fidelity: add a fake-Clock variant that models the wall jump on
    System.Set (guard.go's doc admits fakes don't wire the two together —
    exactly the blind spot that let a field failure mode go unmodeled). Added
    jumpingClock/jumpingSystemClock (fakes_test.go): Set actually moves the
    paired clock's Now(), unlike the plain fakeClock/fakeSystemClock pair —
    see TestJumpingClockModelsSetAsAWallJump.

Related: RTC epic [[gosd-achn]] sidesteps this on RTC boards, but Pi-class
boards have no RTC — this bean is the NTP-only path's correctness story.



## Summary of Changes

All fix-plan todos implemented except "Investigate the field device" (needs
hardware access — deferred to a bench/serial session), which is not blocking:
the fix stands on its own regardless of what that specific device turns out
to have logged.

Scope: every code change is confined to
`cmd/gosd-init/internal/timesync/`, per this task's scope constraint (a
separate stack owns `cmd/gosd` and `internal/pipeline` right now).

- `interfaces.go`: new `SNTPSample` type (Time, Leap, Stratum,
  TransmitTimestamp) and `sntpLeapNotInSync` constant; `NTPClient.Query` now
  returns `SNTPSample` instead of a bare `time.Time`, so validation has
  something to validate.
- `ntpclient.go`: `beevikClient.Query` now calls `ntp.Query`+`Validate`
  (unchanged behavior) but keeps the parsed Leap/Stratum/TransmitTime instead
  of discarding them.
- `timesync.go`: `validateSample` (LI=3, stratum 0/>15, zero transmit
  timestamp) runs in `queryServers` on every response; `Run` logs once when
  `opts.Floor` is disabled; added `Options.PendingConfirmDelay` +
  `DefaultPendingConfirmDelay` (45s) and `pendingConfirmDelay`, used by `Run`'s
  resync loop whenever a step-guard confirmation is outstanding.
- `guard.go`: `stepGuard.check` fast-paths a forward step to at least Floor
  when its own `expected()` is stuck before it (no second-query agreement
  needed), leaving the two-query protection intact for every plausible-clock
  case; the max-step refusal log now names the candidate, expected time, and
  floor (`formatFloor`, printing "none" when disabled).
- `fakes_test.go`: `ntpResult`/`fakeNTPClient` updated for the `SNTPSample`
  return type (existing call sites needed no changes — they get a
  well-formed sample built from `t` for free); added `jumpingClock`/
  `jumpingSystemClock`, a fake pair where `Set` actually jumps the paired
  clock's `Now()`.
- `guard_test.go` (new), `sntp_test.go` (new): direct unit tests for the
  floor fast-path, the "protection intact" case, the refusal log's new
  context, `validateSample`, and the wall-jump fake; `timesync_test.go`:
  end-to-end tests for the floor-disabled boot line and the shortened
  pending-confirm delay.

Verified read-only (not edited, per scope): `internal/pipeline/pipeline.go`
marshals `config.json`'s `BuildTimestamp` from `time.Now().UTC()`
unconditionally — there is no branch in today's build pipeline that can
leave it empty. A floor-less device can only be running an image built by a
gosd release older than this field, not a defect in the current pipeline.

Gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run
./...`, and `GOOS=linux golangci-lint run ./...` all clean.
