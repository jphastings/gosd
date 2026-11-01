package timesync

import (
	"testing"
	"time"
)

func TestStepToFloor(t *testing.T) {
	floor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		now        time.Time
		floor      time.Time
		wantSet    bool
		wantLogged bool
	}{
		{"epoch clock is stepped to the floor", time.Unix(0, 0), floor, true, true},
		{"clock after the floor is untouched", floor.Add(time.Hour), floor, false, false},
		{"clock at the floor is untouched", floor, floor, false, false},
		{"zero floor is untouched", time.Unix(0, 0), time.Time{}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sys := &fakeSystemClock{}
			log := &testLog{}
			StepToFloor(tt.now, tt.floor, sys, log.Printf)
			got := sys.sets()
			if tt.wantSet && (len(got) != 1 || !got[0].Equal(tt.floor)) {
				t.Fatalf("System.Set calls = %v, want exactly [%v]", got, tt.floor)
			}
			if !tt.wantSet && len(got) != 0 {
				t.Fatalf("System.Set calls = %v, want none", got)
			}
			if logged := log.contains("stepped forward"); logged != tt.wantLogged {
				t.Errorf("step logged = %v, want %v: %v", logged, tt.wantLogged, log.snapshot())
			}
		})
	}
}

func TestStepToFloorFailureIsLoggedNotFatal(t *testing.T) {
	sys := &fakeSystemClock{err: errBoom}
	log := &testLog{}
	StepToFloor(time.Unix(0, 0), time.Now(), sys, log.Printf)
	if !log.contains("failed") {
		t.Errorf("failure not logged: %v", log.snapshot())
	}
}
