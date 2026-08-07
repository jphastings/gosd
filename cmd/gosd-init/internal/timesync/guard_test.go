package timesync

import (
	"testing"
	"time"
)

// TestStepGuardFastPathsForwardStepFromPreFloorAnchor is gosd-dqps's fast
// -recovery test: a guard anchored before opts.Floor (exactly what a
// floor-less first sync produces — see Run's "floor is disabled" log)
// must accept a forward step to at least Floor immediately, without
// waiting for a second, agreeing query.
func TestStepGuardFastPathsForwardStepFromPreFloorAnchor(t *testing.T) {
	g := &stepGuard{}
	anchorClock := time.Unix(0, 0)
	bogusFirstSync := time.Unix(1, 0) // an epoch-ish first sync, per the field report
	g.setAnchor(anchorClock, bogusFirstSync)

	opts := Options{
		MaxStep: 1000 * time.Second,
		Floor:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	log := &testLog{}
	deps := Deps{Log: log.Printf}

	old := anchorClock.Add(time.Minute) // barely any time has passed
	realNow := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)

	if !g.check(deps, opts, old, realNow) {
		t.Fatal("a forward step to at least the floor from a pre-floor anchor must be accepted immediately")
	}
	if g.pending != nil {
		t.Error("accepting the fast-pathed step must not leave a pending confirmation")
	}
	if !log.contains("pre-floor anchor") {
		t.Errorf("log missing fast-path explanation: %v", log.snapshot())
	}
}

// TestStepGuardKeepsTwoQueryProtectionForPlausibleClock confirms the
// floor fast-path in TestStepGuardFastPathsForwardStepFromPreFloorAnchor
// doesn't weaken gosd-0esw's protection for the ordinary case: once the
// guard's anchor is itself at or after the floor (a plausible clock), a
// large step still needs a second, agreeing query before being applied.
func TestStepGuardKeepsTwoQueryProtectionForPlausibleClock(t *testing.T) {
	g := &stepGuard{}
	floor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	anchorClock := time.Unix(0, 0)
	g.setAnchor(anchorClock, floor.Add(time.Hour)) // a plausible, post-floor anchor

	opts := Options{
		MaxStep: 1000 * time.Second,
		Floor:   floor,
	}
	log := &testLog{}
	deps := Deps{Log: log.Printf}

	old := anchorClock.Add(time.Minute)
	bigJump := floor.Add(48 * time.Hour) // far beyond MaxStep from expected(old)

	if g.check(deps, opts, old, bigJump) {
		t.Fatal("a large step from a plausible, post-floor anchor must still require confirmation")
	}
	if g.pending == nil {
		t.Fatal("the refused candidate must be remembered as pending")
	}
	if !log.contains("max-step threshold") {
		t.Errorf("log missing the ordinary over-threshold refusal message: %v", log.snapshot())
	}

	// The confirming query, close enough to bigJump, is now accepted.
	confirming := bigJump.Add(time.Hour)
	old2 := old.Add(time.Hour)
	if !g.check(deps, opts, old2, confirming) {
		t.Fatal("a confirming second query must be accepted")
	}
}

// TestStepGuardRefusalLogIncludesExpectedAndFloor checks the refusal
// message carries enough context (candidate, expected, floor) to be
// self-diagnosing in the field, unlike the bare "which exceeds the...
// threshold" line from JP's field report.
func TestStepGuardRefusalLogIncludesExpectedAndFloor(t *testing.T) {
	g := &stepGuard{}
	floor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g.setAnchor(time.Unix(0, 0), floor.Add(time.Hour))

	opts := Options{MaxStep: 1000 * time.Second, Floor: floor}
	log := &testLog{}
	deps := Deps{Log: log.Printf}

	g.check(deps, opts, time.Unix(60, 0), floor.Add(48*time.Hour))

	for _, want := range []string{"candidate", "expected", "floor"} {
		if !log.contains(want) {
			t.Errorf("refusal log missing %q context: %v", want, log.snapshot())
		}
	}
}

// TestStepGuardRefusalLogReportsNoFloorWhenDisabled confirms the refusal
// message doesn't print Go's zero-time string when Floor is disabled —
// it must read "none" instead, per formatFloor.
func TestStepGuardRefusalLogReportsNoFloorWhenDisabled(t *testing.T) {
	g := &stepGuard{}
	g.setAnchor(time.Unix(0, 0), time.Unix(1700000000, 0))

	opts := Options{MaxStep: 1000 * time.Second} // Floor left zero (disabled)
	log := &testLog{}
	deps := Deps{Log: log.Printf}

	g.check(deps, opts, time.Unix(60, 0), time.Unix(1700000000, 0).Add(48*time.Hour))

	if !log.contains("floor none") {
		t.Errorf("refusal log should report a disabled floor as \"none\": %v", log.snapshot())
	}
}

// TestJumpingClockModelsSetAsAWallJump exercises the fake introduced for
// gosd-dqps's "fakes don't wire System.Set to Clock.Now" blind spot
// (guard.go's own doc admits it): once jumpingSystemClock.Set is called,
// the paired jumpingClock's Now() must reflect the new time immediately,
// the way production's real Clock/SystemClock pair does via the OS wall
// clock.
func TestJumpingClockModelsSetAsAWallJump(t *testing.T) {
	clock := newJumpingClock(time.Unix(0, 0))
	sys := &jumpingSystemClock{clock: clock}

	newTime := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	if err := sys.Set(newTime); err != nil {
		t.Fatalf("Set returned an error: %v", err)
	}

	if got := clock.Now(); !got.Equal(newTime) {
		t.Fatalf("Clock.Now() after Set = %v, want %v (the jump)", got, newTime)
	}
	if got := sys.sets(); len(got) != 1 || !got[0].Equal(newTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v", got, newTime)
	}
}
