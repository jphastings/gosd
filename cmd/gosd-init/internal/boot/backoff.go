package boot

import "time"

// Boot-sequence-mandated restart backoff bounds: restart /app on exit with
// exponential backoff capped at 10s. The doubling/capping engine itself is
// childbackoff.Backoff (bean gosd-gkbi consolidated /app's own copy onto
// it); these constants are boot's own choice of that engine's base/max.
const (
	DefaultBackoffBase = 1 * time.Second
	DefaultBackoffCap  = 10 * time.Second

	// StableRunThreshold is how long /app must run before a subsequent exit
	// is treated as a fresh failure rather than a continuation of a crash
	// loop, resetting the backoff delay back to DefaultBackoffBase. This
	// isn't spelled out by the boot sequence itself; it's a deliberate,
	// commonly-used interpretation (systemd, Kubernetes) of "exponential
	// backoff" so that a device which crash-loops once early on doesn't
	// stay slow to restart for the rest of its uptime.
	StableRunThreshold = 30 * time.Second
)
