---
# gosd-dqps
title: 'timesync: RTC-less boot must converge via NTP alone — floor silently inactive + bogus first sync causes 56-year step refusals'
status: todo
type: bug
priority: high
created_at: 2026-08-07T13:04:22Z
updated_at: 2026-08-07T13:04:22Z
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
[ ] SNTP response validation in queryServers: reject LI=3 (alarm/unsync),
    stratum 0 or >15, zero transmit timestamp. An unsynced home router is
    the expected bench case, not an exotic attack.
[ ] Floor must never be silently absent: gosd build fails (or loudly warns)
    if it would bake an empty buildTimestamp; gosd-init logs ONE boot line
    when the floor is disabled so this state is visible in the field.
[ ] Fast recovery from a provably-bogus clock: when expected() < Floor, a
    forward step to ≥ Floor needs NO two-query agreement — the clock cannot
    legitimately read pre-build. (Keeps full two-query protection for all
    plausible-clock cases; gosd-0esw's threat model intact.)
[ ] Refusal log gains expected/floor context so field lines like JP's are
    self-diagnosing.
[ ] Pending-confirmation resyncs shouldn't wait a full ResyncEvery: schedule
    the confirming query sooner (e.g. 30–60s) to bound wrong-clock time.
[ ] Test-fidelity: add a fake-Clock variant that models the wall jump on
    System.Set (guard.go's doc admits fakes don't wire the two together —
    exactly the blind spot that let a field failure mode go unmodeled).

Related: RTC epic [[gosd-achn]] sidesteps this on RTC boards, but Pi-class
boards have no RTC — this bean is the NTP-only path's correctness story.
