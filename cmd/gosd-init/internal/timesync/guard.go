package timesync

import "time"

// stepAgreementTolerance bounds how closely two consecutive over-MaxStep
// candidates must agree to be treated as confirming each other — see
// pendingStep.agrees. Deliberately loose next to NTP's usual sub-second
// dispersion, but tight next to MaxStep itself: two independently forged
// responses landing within it, in two separate query rounds, is a
// materially higher bar than the single racing packet gosd-0esw's threat
// model describes ("an on-path/LAN attacker races a forged response").
const stepAgreementTolerance = 30 * time.Second

// stepGuard implements the post-first-sync half of gosd-0esw's fix: ntpd's
// classic max-step/panic threshold, with a periodic-retry escape hatch so
// a genuinely large but legitimate step (a device that was powered off for
// a long time) isn't wedged forever rather than eventually accepted. One
// stepGuard is created per Run call and threaded through the first sync
// and every scheduled resync after it (see Run), so:
//
//   - a candidate is only ever confirmed against the immediately
//     preceding *rejected* candidate, never an older or an
//     already-applied one (see check and pendingStep.agrees);
//   - "how large a step is this" is measured against wherever the clock
//     should currently read given the most recently *applied* sync, not
//     against a fresh read of the system clock — SystemClock only ever
//     exposes Set, deliberately (see interfaces.go), so this tracks the
//     mapping between deps.Clock's timeline and the system clock's value
//     itself, entirely in this package's own state (see setAnchor and
//     expected). That also keeps this pure and fake-driven: production's
//     Clock is real wall time, which genuinely does move when
//     System.Set changes it, but a test fake has no reason to wire the
//     two together, and correctness here shouldn't depend on whether it
//     does.
//
// Not used for the very first sync (syncUntilSuccess only calls
// setAnchor, never check): before that there's no trustworthy "current
// clock" reading to measure a step against at all — the board has no
// battery-backed RTC, so the clock starts every boot near the Unix
// epoch, and *any* correct time is a huge step away from that. The floor
// check (see checkFloor) guards the first sync instead.
type stepGuard struct {
	// anchorClock/anchorTime record deps.Clock's reading at the moment of
	// the most recently applied sync, and what the system clock was set
	// to at that moment. Together with a later deps.Clock.Now() reading,
	// that's enough to estimate what the system clock should currently
	// read (see expected) without ever reading it back directly.
	anchorClock time.Time
	anchorTime  time.Time

	// pending is the most recent over-threshold candidate this stepGuard
	// refused, kept so the *next* resync can decide whether it agrees —
	// nil whenever there's no unconfirmed refusal outstanding.
	pending *pendingStep
}

// pendingStep is one refused over-threshold candidate.
type pendingStep struct {
	candidate time.Time // the NTP result that was refused
	old       time.Time // deps.Clock.Now() when it was refused
}

// setAnchor records that, as of clockNow (a deps.Clock reading), the
// system clock was set to appliedTime — called after every sync this
// package actually applies, first sync included, so expected always
// reflects the most recent one.
func (g *stepGuard) setAnchor(clockNow, appliedTime time.Time) {
	g.anchorClock = clockNow
	g.anchorTime = appliedTime
}

// expected estimates what the system clock should currently read, given
// clockNow (a deps.Clock reading taken at the same moment) and the most
// recent setAnchor call: the clock ticks at the normal rate from wherever
// it was last set, so the elapsed deps.Clock duration since the anchor is
// exactly how far the system clock should have advanced too.
func (g *stepGuard) expected(clockNow time.Time) time.Time {
	return g.anchorTime.Add(clockNow.Sub(g.anchorClock))
}

// check decides whether newTime (queried at old, a deps.Clock reading)
// should be applied: within opts.MaxStep of expected(old), it's an
// ordinary step and always allowed; beyond it, it's allowed only once an
// immediately following call reports a candidate that agrees with the one
// this call refuses (see pendingStep.agrees) — otherwise it's refused and
// logged, and remembered as the new pending candidate for the next call to
// judge. opts.MaxStep <= 0 disables the guard entirely (every candidate
// allowed, pending cleared).
func (g *stepGuard) check(deps Deps, opts Options, old, newTime time.Time) bool {
	if opts.MaxStep <= 0 {
		g.pending = nil
		return true
	}

	step := newTime.Sub(g.expected(old))
	if step < 0 {
		step = -step
	}
	if step <= opts.MaxStep {
		g.pending = nil
		return true
	}

	prev := g.pending
	if prev != nil && prev.agrees(old, newTime) {
		deps.Log("NTP resync step of %s confirmed by a second, agreeing query; applying", step)
		g.pending = nil
		return true
	}

	deps.Log("NTP resync wants to step the clock by %s, which exceeds the %s max-step threshold; refusing until a second query agrees", step, opts.MaxStep)
	g.pending = &pendingStep{candidate: newTime, old: old}
	return false
}

// agrees reports whether a new candidate (queried at old, a deps.Clock
// reading) is consistent with p: however wrong the clock's epoch is, it
// still ticks at the normal rate, so a candidate's movement between two
// queries should track the real (deps.Clock) time that elapsed between
// them — a genuinely large-but-legitimate offset (a long-powered-off
// device) stays essentially constant across queries taken close together,
// which is exactly this check, algebraically: it holds when
// (newTime-p.candidate) is within stepAgreementTolerance of (old-p.old).
func (p pendingStep) agrees(old, newTime time.Time) bool {
	elapsed := old.Sub(p.old)
	moved := newTime.Sub(p.candidate)
	diff := moved - elapsed
	if diff < 0 {
		diff = -diff
	}
	return diff <= stepAgreementTolerance
}
