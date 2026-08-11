package boot

import "sync"

// maxStashedResults bounds how many reaped-but-unclaimed exit statuses the
// reaper remembers, so PID 1 doesn't accumulate one map entry per orphaned
// grandchild for the lifetime of the device.
const maxStashedResults = 64

// reaper matches reaped pids to whoever is waiting for their exit status.
// The Linux half — the SIGCHLD-driven wait4(-1, ...) loop that calls
// deliver — lives in platform_linux.go; this half is plain bookkeeping so it
// can be tested anywhere.
//
// Every reaped pid's status is stashed, not just the ones a Wait is already
// parked on: /app can exit before the supervisor gets as far as calling Wait
// on the pid Start just returned, and a status discarded in that window
// leaves Wait blocked forever with the app never restarted (bean gosd-1t0q).
//
// Wait itself never returns an error (see Wait's doc): once wait4 has
// confirmed a pid is reaped, the only way deliver is ever called, getting
// its ExitStatus cannot fail. Wait still returns one to satisfy the general
// Reaper interface fakes use in tests, but the real implementation always
// returns nil for it.
type reaper struct {
	mu      sync.Mutex
	waiters map[int]chan ExitStatus
	results map[int]ExitStatus
	// stashed holds the pids in results, oldest first, so the stash can be
	// pruned in reaping order.
	stashed []int
}

func newReaper() *reaper {
	return &reaper{
		waiters: make(map[int]chan ExitStatus),
		results: make(map[int]ExitStatus),
	}
}

// deliver hands pid's exit status to a parked Wait, or stashes it for a Wait
// that hasn't happened yet.
func (r *reaper) deliver(pid int, status ExitStatus) {
	r.mu.Lock()
	ch, waiting := r.waiters[pid]
	if waiting {
		delete(r.waiters, pid)
	} else {
		r.stash(pid, status)
	}
	r.mu.Unlock()

	if waiting {
		ch <- status
	}
}

// stash records pid's status until someone claims it, evicting the oldest
// entry once the stash is full. Eviction can't lose a supervised child's
// status in practice: gosd-init supervises a small, fixed set of children
// (the app, plus gosd-shipped system services like cloudflared and
// tailscale-funnel — a narrow carve-out to the original single-child
// design, see gosd-oyhi) and calls
// Wait on each one as soon as its own Start returns, so an eviction would
// need maxStashedResults *other* pids (grandchildren orphaned to PID 1) to
// be reaped inside that pid's own Start-to-Wait window — the argument holds
// per pid, not on the total number of children gosd-init happens to be
// supervising at once. Callers hold r.mu.
func (r *reaper) stash(pid int, status ExitStatus) {
	if _, known := r.results[pid]; !known {
		r.stashed = append(r.stashed, pid)
	}
	r.results[pid] = status

	if len(r.stashed) > maxStashedResults {
		delete(r.results, r.stashed[0])
		r.stashed = r.stashed[1:]
	}
}

// claim takes pid's stashed status, if one arrived already. Callers hold
// r.mu.
func (r *reaper) claim(pid int) (ExitStatus, bool) {
	res, ok := r.results[pid]
	if !ok {
		return ExitStatus{}, false
	}
	delete(r.results, pid)
	for i, stashed := range r.stashed {
		if stashed == pid {
			r.stashed = append(r.stashed[:i], r.stashed[i+1:]...)
			break
		}
	}
	return res, true
}

// Wait blocks until pid has been reaped and returns everything the reaper
// knows about how it died.
func (r *reaper) Wait(pid int) (ExitStatus, error) {
	r.mu.Lock()
	if res, ok := r.claim(pid); ok {
		r.mu.Unlock()
		return res, nil
	}
	ch := make(chan ExitStatus, 1)
	r.waiters[pid] = ch
	r.mu.Unlock()

	res := <-ch
	return res, nil
}
