package cloudflared

import "time"

// Bounds and reset threshold for the cloudflared restart backoff (locked
// decision, bean gosd-uj36): a single auxiliary process, not the boot
// sequence's own /app — capped low enough (30s) that a permanently broken
// tunnel logs no more than ~2 lines/minute to the 115200 serial console,
// with no jitter needed since cloudflared is the only thing gosd-init ever
// starts here (jitter exists to spread a fleet's simultaneous retries
// across a shared resource, e.g. netup.Backoff's DHCP server; cloudflared
// itself already jitters its own edge reconnects internally). The doubling/
// capping engine itself lives in childbackoff (bean gosd-wxjy); these
// constants are cloudflared's own choice of that engine's base/max, plus
// the stable-run threshold that decides when runOnce resets it.
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 30 * time.Second

	// StableAfter is how long cloudflared must run before its next exit
	// resets the backoff back to DefaultBackoffBase, mirroring
	// boot.Supervisor's StableRunThreshold (same rationale: a device that
	// crash-loops once early on shouldn't stay slow to restart for the
	// rest of its uptime).
	StableAfter = 30 * time.Second
)
