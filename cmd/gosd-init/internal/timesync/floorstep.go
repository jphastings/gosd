package timesync

import "time"

// StepToFloor raises the system clock to floor when it reads earlier —
// the epoch on a board with no (or a flat-battery) RTC — so certificate
// validity checks can pass before SNTP answers. A later clock, or a zero
// floor, is left alone, so a clock HCTOSYS already seeded is never pulled
// back. It touches neither the RTC nor the time-synced marker: floor is
// the image's build time, not a trustworthy reading.
func StepToFloor(now time.Time, floor time.Time, system SystemClock, log func(format string, args ...any)) {
	if floor.IsZero() || !now.Before(floor) {
		return
	}
	if err := system.Set(floor); err != nil {
		log("stepping the clock to this build's time failed: %v", err)
		return
	}
	log("clock read %s, before this build; stepped forward to the build time %s until NTP syncs",
		now.Format(time.RFC3339), floor.Format(time.RFC3339))
}
