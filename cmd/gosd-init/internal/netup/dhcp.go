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

	for {
		lease, err := deps.DHCP.Request(ctx, iface)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			delay := backoff.Next()
			deps.Log("DHCP discovery on %s failed: %v; retrying in %s", iface, err, delay)
			select {
			case <-ctx.Done():
				return nil
			case <-deps.Clock.After(delay):
				continue
			}
		}

		backoff.Reset()
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
// It returns nil only when ctx is cancelled.
func maintainLease(ctx context.Context, deps Deps, iface string, lease *Lease, onLease func(*Lease)) error {
	for {
		if !waitUntil(ctx, deps.Clock, lease.ObtainedAt.Add(lease.RenewAfter)) {
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

		if !waitUntil(ctx, deps.Clock, lease.ObtainedAt.Add(lease.RebindAfter)) {
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

// waitUntil blocks until deps.Clock reaches target, ctx is cancelled, or
// (if target is already in the past) returns immediately. It reports
// whether it returned because target was reached (true) as opposed to ctx
// being cancelled (false).
func waitUntil(ctx context.Context, clock Clock, target time.Time) bool {
	d := target.Sub(clock.Now())
	if d < 0 {
		d = 0
	}
	select {
	case <-ctx.Done():
		return false
	case <-clock.After(d):
		return true
	}
}
