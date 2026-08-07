package timesync

import (
	"testing"
	"time"
)

// TestValidateSampleAcceptsWellFormedSample confirms the ordinary,
// trustworthy case: a plausible stratum, no leap warning, and a nonzero
// transmit timestamp all pass.
func TestValidateSampleAcceptsWellFormedSample(t *testing.T) {
	sample := SNTPSample{Time: time.Unix(1700000000, 0), Stratum: 2, TransmitTimestamp: time.Unix(1700000000, 0)}
	if err := validateSample(sample); err != nil {
		t.Errorf("validateSample rejected a well-formed sample: %v", err)
	}
}

// TestValidateSampleRejectsUnsynchronizedSignatures is gosd-dqps's core
// SNTP-validation test: each of these is the signature of a
// fresh-booted, not-yet-synced server (the expected bench case per the
// bean, not an exotic attack) and must be rejected outright.
func TestValidateSampleRejectsUnsynchronizedSignatures(t *testing.T) {
	validTime := time.Unix(1700000000, 0)

	cases := map[string]SNTPSample{
		"leap indicator 3 (not in sync)": {Time: validTime, Leap: sntpLeapNotInSync, Stratum: 2, TransmitTimestamp: validTime},
		"stratum 0 (kiss of death)":      {Time: validTime, Stratum: 0, TransmitTimestamp: validTime},
		"stratum 16 (invalid)":           {Time: validTime, Stratum: 16, TransmitTimestamp: validTime},
		"stratum 255 (invalid)":          {Time: validTime, Stratum: 255, TransmitTimestamp: validTime},
		"zero transmit timestamp":        {Time: validTime, Stratum: 2, TransmitTimestamp: time.Time{}},
	}

	for name, sample := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateSample(sample); err == nil {
				t.Errorf("validateSample accepted an untrustworthy sample (%s)", name)
			}
		})
	}
}

// TestRunSkipsUnsynchronizedServerAndTriesTheNext reproduces the bean's
// "classic source" scenario end to end: the first configured server is a
// freshly booted, unsynchronized router (LI=3), and Run must treat it
// exactly like a server that didn't answer at all — skipping straight to
// the next server in the same round rather than applying its bogus time.
func TestRunSkipsUnsynchronizedServerAndTriesTheNext(t *testing.T) {
	clock := newFakeClock(time.Unix(0, 0))
	ntp := newFakeNTPClient()
	syncedTime := time.Unix(1700000000, 0)
	ntp.script("unsynced-router", ntpResult{sample: &SNTPSample{
		Time:              time.Unix(1, 0), // era-zero-ish, per the field report
		Leap:              sntpLeapNotInSync,
		Stratum:           1,
		TransmitTimestamp: time.Unix(1, 0),
	}})
	ntp.script("good", ntpResult{t: syncedTime})
	sys := &fakeSystemClock{}
	up := &flag{}
	up.set(true)
	log := &testLog{}
	deps, _ := newTestDeps(clock, ntp, sys, up, log)

	stop := make(chan struct{})
	defer close(stop)
	go Run(deps, defaultOptions([]string{"unsynced-router", "good"}, stop))

	deadline := time.Now().Add(2 * time.Second)
	for len(sys.sets()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := sys.sets(); len(got) != 1 || !got[0].Equal(syncedTime) {
		t.Fatalf("System.Set calls = %v, want exactly one call with %v", got, syncedTime)
	}
	if !log.contains("untrustworthy") {
		t.Errorf("log missing the untrustworthy-response message: %v", log.snapshot())
	}
}
