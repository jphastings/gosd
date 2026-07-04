package timesync

import (
	"time"

	"github.com/beevik/ntp"
)

// beevikClient implements NTPClient using the locked github.com/beevik/ntp
// dependency. Unlike SystemClock.Set (settimeofday), querying an NTP
// server is a plain UDP round-trip with no OS-specific syscall involved,
// so this needs no platform_linux.go/platform_other.go split: it's
// constructed directly by both NewPlatform implementations.
type beevikClient struct{}

func newBeevikClient() NTPClient { return beevikClient{} }

// Query returns ntp.Time's already-corrected result: local system time
// plus the measured clock offset from server.
func (beevikClient) Query(server string) (time.Time, error) {
	return ntp.Time(server)
}
