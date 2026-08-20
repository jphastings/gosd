package netup

import (
	"testing"
	"time"
)

// maxDHCPLeaseTime is what a server sends when it offers the RFC 2131
// Section 3.3 "infinite" lease: 0xffffffff seconds.
const maxDHCPLeaseTime = 4294967295 * time.Second

func TestSaneLeaseTimers(t *testing.T) {
	tests := []struct {
		name                           string
		renew, rebind, expire          time.Duration
		wantRenew, wantRebind, wantExp time.Duration
	}{
		{
			name:       "an ordinary lease is left alone",
			renew:      30 * time.Minute,
			rebind:     52*time.Minute + 30*time.Second,
			expire:     time.Hour,
			wantRenew:  30 * time.Minute,
			wantRebind: 52*time.Minute + 30*time.Second,
			wantExp:    time.Hour,
		},
		{
			name:       "no lease time at all falls back to a typical lease",
			wantRenew:  30 * time.Minute,
			wantRebind: 52*time.Minute + 30*time.Second,
			wantExp:    time.Hour,
		},
		{
			name:       "a server demanding renewal every second gets the floor",
			renew:      time.Second,
			rebind:     2 * time.Second,
			expire:     3 * time.Second,
			wantRenew:  minLeaseTimer,
			wantRebind: minLeaseTimer,
			wantExp:    minLeaseTimer,
		},
		{
			name:       "an infinite lease is capped rather than trusted forever",
			expire:     maxDHCPLeaseTime,
			wantRenew:  12 * time.Hour,
			wantRebind: 21 * time.Hour,
			wantExp:    maxLeaseTimer,
		},
		{
			name:       "a rebind time that overflowed to negative is recomputed",
			renew:      maxDHCPLeaseTime / 2,
			rebind:     -9 * 24 * time.Hour,
			expire:     maxDHCPLeaseTime,
			wantRenew:  maxLeaseTimer,
			wantRebind: maxLeaseTimer,
			wantExp:    maxLeaseTimer,
		},
		{
			name:       "rebinding before renewal is reordered",
			renew:      2 * time.Hour,
			rebind:     time.Hour,
			expire:     3 * time.Hour,
			wantRenew:  2 * time.Hour,
			wantRebind: 2 * time.Hour,
			wantExp:    3 * time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			renew, rebind, expire := saneLeaseTimers(tc.renew, tc.rebind, tc.expire)
			if renew != tc.wantRenew || rebind != tc.wantRebind || expire != tc.wantExp {
				t.Errorf("saneLeaseTimers(%s, %s, %s) = (%s, %s, %s), want (%s, %s, %s)",
					tc.renew, tc.rebind, tc.expire, renew, rebind, expire,
					tc.wantRenew, tc.wantRebind, tc.wantExp)
			}
			if renew < minLeaseTimer || rebind < renew || expire < rebind || expire > maxLeaseTimer {
				t.Errorf("timers (%s, %s, %s) break minLeaseTimer <= T1 <= T2 <= lease <= maxLeaseTimer",
					renew, rebind, expire)
			}
		})
	}
}
