package netup

import (
	"context"
	"fmt"
	"time"
)

// RunDHCP performs DHCPv4 lease acquisition and maintenance on iface:
// Discover/Request retried forever with jittered backoff (the cable, or —
// for a future WiFi caller — the wireless association, may complete after
// this is called), then T1/T2-driven renewal for as long as ctx isn't
// cancelled. onLease is invoked with every lease obtained, including
// renewals, so the caller can (re)apply the address/route/DNS.
//
// Per the bean's "forever" requirement, a failed Discover/Request or a
// lost lease is never fatal: it always loops back and retries. As a
// result this only ever returns (with nil) once ctx is cancelled — a
// graceful stop, e.g. the link went down. It still returns an error
// (rather than nothing) so a caller-side change that makes some future
// failure mode fatal doesn't require an API change here.
//
// Exported, rather than kept private to Ethernet bring-up, because a later
// WiFi bean is expected to call it directly once its nl80211 association
// brings the wifi interface's carrier up: DHCP itself doesn't care whether
// the underlying medium is wired or wireless, only that iface exists and
// is up.
func RunDHCP(ctx context.Context, deps Deps, iface string, onLease func(*Lease)) error {
	backoff := deps.NewBackoff()
	status := newRetryStatus(deps.Clock)

	for {
		lease, err := deps.DHCP.Request(ctx, iface)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			delay := backoff.Next()
			status.fail(deps, iface, err, delay)
			select {
			case <-ctx.Done():
				return nil
			case <-deps.Clock.After(delay):
				continue
			}
		}

		backoff.Reset()
		status.reset()
		onLease(lease)

		err = maintainLease(ctx, deps, iface, lease, onLease)
		if err == nil {
			// ctx was cancelled: a graceful stop, not a failure.
			return nil
		}
		deps.Log("lease on %s lost: %v; restarting discovery", iface, err)
	}
}

// maintainLease renews lease at T1, retries at T2 (rebinding) if that
// renewal failed, and reports lease loss (a non-nil error) if rebinding
// also fails — the caller (RunDHCP) then restarts discovery from scratch.
// It returns nil only when ctx is cancelled. T1 and T2 come from the
// server, so both are taken through leaseSchedule (see leasetimes.go)
// rather than used as sent.
func maintainLease(ctx context.Context, deps Deps, iface string, lease *Lease, onLease func(*Lease)) error {
	schedule := newLeaseSchedule(deps, iface)

	for {
		renewAt, rebindAt := schedule.times(lease)

		if !waitForLeaseTimer(ctx, deps.Clock, renewAt) {
			return nil
		}

		renewed, err := deps.DHCP.Renew(ctx, iface, lease)
		if err == nil {
			lease = renewed
			onLease(lease)
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		deps.Log("renewing lease on %s failed: %v; will retry at rebind", iface, err)

		if !waitForLeaseTimer(ctx, deps.Clock, rebindAt) {
			return nil
		}

		rebound, err := deps.DHCP.Renew(ctx, iface, lease)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("renew and rebind both failed: %w", err)
		}
		lease = rebound
		onLease(lease)
	}
}

// waitForLeaseTimer blocks until deps.Clock reaches target or ctx is
// cancelled, and reports whether it returned because target was reached
// (true) as opposed to ctx being cancelled (false). It never returns
// sooner than minLeaseTimer from now, whatever target says: a deadline
// already in the past — from lease timers leaseSchedule couldn't make
// sense of, or a clock that timesync stepped forwards after the lease was
// obtained — must not turn lease maintenance into a tight DHCP loop
// against the server (bean gosd-8lw0).
func waitForLeaseTimer(ctx context.Context, clock Clock, target time.Time) bool {
	d := target.Sub(clock.Now())
	if d < minLeaseTimer {
		d = minLeaseTimer
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(d):
		return true
	}
}
