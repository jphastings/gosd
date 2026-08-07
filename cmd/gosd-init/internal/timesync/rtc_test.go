package timesync

import (
	"testing"
	"time"
)

// TestRunWritesRTCAfterSuccessfulSync is gosd-lx8g's core behavior: the
// very first successful sync writes the corrected time back to the RTC,
// not just the system clock.
func TestRunWritesRTCAfterSuccessfulSync(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	syncedTime := time.Unix(1700000000, 0)
	ntp.script("ntp1", ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	rtc := &fakeRTC{}
	deps.RTC = rtc

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for len(rtc.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := rtc.sets(); len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("RTC.Set calls = %v, want exactly one call with %v", got, syncedTime)
	}
}

// TestRunWritesRTCAfterConfirmedLargeStep confirms the RTC write-back
// isn't limited to the first sync: once a resync's over-threshold
// candidate is confirmed by a second, agreeing query (see
// TestRunRefusesOverThresholdStepUntilSecondQueryAgrees), that step lands
// on the RTC too — but the intermediate candidate the guard refused never
// does.
func TestRunWritesRTCAfterConfirmedLargeStep(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	bigJump1 := first.Add(48 * time.Hour)
	bigJump2 := bigJump1.Add(time.Hour)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: bigJump1}, ntpResult{t: bigJump2})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	rtc := &fakeRTC{}
	deps.RTC = rtc

	stop := make(chan struct{})
	defer close(stop)
	opts := defaultOptions([]string{"ntp1"}, stop)
	opts.MaxStep = 1000 * time.Second

	go Run(deps, opts)

	deadline := time.Now().Add(2 * time.Second)
	for len(rtc.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := rtc.sets(); len(got) != 1 || !got[0].Equal(first) {
		t.Fatalf("RTC.Set after first sync = %v, want exactly one call with %v", got, first)
	}

	if !waitForPending(clock, 1) {
		t.Fatal("first resync timer was never registered")
	}
	clock.Advance(time.Hour) // bigJump1: over threshold, refused

	deadline = time.Now().Add(2 * time.Second)
	for !log.contains("max-step threshold") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := rtc.sets(); len(got) != 1 {
		t.Fatalf("RTC.Set calls = %v, a refused candidate must never reach the RTC", got)
	}

	if !waitForPending(clock, 1) {
		t.Fatal("second resync timer was never registered")
	}
	clock.Advance(time.Hour) // bigJump2: confirms bigJump1

	deadline = time.Now().Add(2 * time.Second)
	for len(rtc.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got := rtc.sets()
	if len(got) != 2 || !got[1].Equal(bigJump2) {
		t.Fatalf("RTC.Set calls = %v, want a second call with %v once the confirming query agreed", got, bigJump2)
	}
}

// TestRunDoesNotWriteRTCBeforeFirstSyncSucceeds confirms a round where no
// server answered never reaches the RTC.
func TestRunDoesNotWriteRTCBeforeFirstSyncSucceeds(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	syncedTime := time.Unix(1700000000, 0)
	ntp.script("ntp1", ntpResult{err: errBoom}, ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	rtc := &fakeRTC{}
	deps.RTC = rtc

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	if !waitForPending(clock, 1) {
		t.Fatal("no pending backoff timer after the failed attempt")
	}
	if got := rtc.sets(); len(got) != 0 {
		t.Fatalf("RTC.Set called %v after a failed NTP round, want none", got)
	}

	clock.Advance(10 * time.Second) // exceeds any backoff delay scripted

	deadline := time.Now().Add(2 * time.Second)
	for len(rtc.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := rtc.sets(); len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("RTC.Set calls = %v, want exactly one call with %v", got, syncedTime)
	}
}

// TestRunDoesNotWriteRTCWhenSystemClockSetFails confirms stepClock's
// early return on a failed System.Set also skips the RTC write: the two
// clocks must never disagree about whether a step actually landed.
func TestRunDoesNotWriteRTCWhenSystemClockSetFails(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	ntp.script("ntp1", ntpResult{t: time.Unix(1700000000, 0)})
	sys := &fakeSystemClock{err: errBoom}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	rtc := &fakeRTC{}
	deps.RTC = rtc

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for !log.contains("setting system clock failed") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !log.contains("setting system clock failed") {
		t.Fatal("System.Set failure was never logged")
	}
	if got := rtc.sets(); len(got) != 0 {
		t.Fatalf("RTC.Set called %v after a failed System.Set, want none", got)
	}
}

// TestRunSkipsRTCWriteSilentlyWhenAbsent confirms a board with no RTC at
// all — ErrRTCNotPresent, the sentinel a real platform_linux.go
// implementation reports (see unixRTC) — produces no RTC-related log
// line whatsoever, on the first sync or a resync after it.
func TestRunSkipsRTCWriteSilentlyWhenAbsent(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	second := first.Add(time.Hour + 30*time.Second)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: second})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	rtc := &fakeRTC{err: ErrRTCNotPresent}
	deps.RTC = rtc

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for len(rtc.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !waitForPending(clock, 1) {
		t.Fatal("resync timer was never registered")
	}
	clock.Advance(time.Hour)

	deadline = time.Now().Add(2 * time.Second)
	for len(rtc.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := rtc.sets(); len(got) != 2 {
		t.Fatalf("RTC.Set calls = %v, want exactly 2 attempts (both silently absent)", got)
	}
	if log.contains("RTC") {
		t.Errorf("a board with no RTC must never log anything about it: %v", log.snapshot())
	}
}

// TestRunLogsRTCWriteFailureOnlyOnce confirms a persistently failing RTC
// write produces exactly one boot-log line, not one per sync, mirroring
// how Run's "floor is disabled" line logs once rather than per attempt.
func TestRunLogsRTCWriteFailureOnlyOnce(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	first := time.Unix(1700000000, 0)
	second := first.Add(time.Hour + 30*time.Second)
	ntp.script("ntp1", ntpResult{t: first}, ntpResult{t: second})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)
	rtc := &fakeRTC{err: errBoom}
	deps.RTC = rtc

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"ntp1"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for len(rtc.sets()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !waitForPending(clock, 1) {
		t.Fatal("resync timer was never registered")
	}
	clock.Advance(time.Hour)

	deadline = time.Now().Add(2 * time.Second)
	for len(rtc.sets()) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := rtc.sets(); len(got) != 2 {
		t.Fatalf("RTC.Set calls = %v, want exactly 2 attempts", got)
	}
	if n := log.count("writing system time to the RTC failed"); n != 1 {
		t.Errorf("RTC write-failure warning logged %d times, want exactly 1: %v", n, log.snapshot())
	}
}
