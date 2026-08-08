package tsfunnel

import "time"

// Bounds and reset threshold for the tailscale-funnel restart backoff
// (locked decision, bean gosd-e3mm): identical to cloudflared's own policy
// — a single auxiliary process, capped low enough (30s) that a permanently
// broken shim logs no more than ~2 lines/minute to the 115200 serial
// console, with no jitter (this is the only shim gosd-init starts here).
// The doubling/capping engine itself lives in childbackoff (bean
// gosd-wxjy); these constants are this package's own choice of that
// engine's base/max, plus the stable-run threshold that decides when
// runOnce resets it.
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 30 * time.Second

	// StableAfter is how long the shim must run before its next exit
	// resets the backoff back to DefaultBackoffBase, mirroring
	// cloudflared.StableAfter (same rationale: a device that crash-loops
	// once early on shouldn't stay slow to restart for the rest of its
	// uptime).
	StableAfter = 30 * time.Second
)
