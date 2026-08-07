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

// Query mirrors ntp.Time's request/validate/correct sequence (query,
// then r.Validate() for the vendor library's own structural and
// freshness/dispersion checks) but keeps the parsed *ntp.Response's
// Leap, Stratum and raw transmit time around instead of discarding them,
// so validateSample can run this package's own gosd-dqps checks against
// them too — see SNTPSample's doc for why that's not just belt-and-braces.
func (beevikClient) Query(server string) (SNTPSample, error) {
	r, err := ntp.Query(server)
	if err != nil {
		return SNTPSample{}, err
	}
	if err := r.Validate(); err != nil {
		return SNTPSample{}, err
	}
	return SNTPSample{
		Time:              time.Now().Add(r.ClockOffset),
		Leap:              uint8(r.Leap),
		Stratum:           r.Stratum,
		TransmitTimestamp: r.Time,
	}, nil
}
