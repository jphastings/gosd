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
type reaper struct {
	mu      sync.Mutex
	waiters map[int]chan waitResult
	results map[int]waitResult
	// stashed holds the pids in results, oldest first, so the stash can be
	// pruned in reaping order.
	stashed []int
}

// waitResult carries only an exit status, not an error: once wait4 has
// confirmed a pid is reaped (the only way deliver is ever called), getting
// its exit status cannot itself fail. Wait still returns an error to satisfy
// the general Reaper interface fakes use in tests, but the real
// implementation always returns nil for it.
type waitResult struct {
	status int
}

func newReaper() *reaper {
	return &reaper{
		waiters: make(map[int]chan waitResult),
		results: make(map[int]waitResult),
	}
}

// deliver hands pid's exit status to a parked Wait, or stashes it for a Wait
// that hasn't happened yet.
func (r *reaper) deliver(pid, status int) {
	r.mu.Lock()
	ch, waiting := r.waiters[pid]
	if waiting {
		delete(r.waiters, pid)
	} else {
		r.stash(pid, status)
	}
	r.mu.Unlock()

	if waiting {
		ch <- waitResult{status: status}
	}
}

// stash records pid's status until someone claims it, evicting the oldest
// entry once the stash is full. Eviction can't lose the app's status in
// practice: gosd-init supervises a single child at a time and calls Wait on
// it as soon as Start returns, so an eviction would need maxStashedResults
// *other* pids (grandchildren orphaned to PID 1) to be reaped inside that
// window. Callers hold r.mu.
func (r *reaper) stash(pid, status int) {
	if _, known := r.results[pid]; !known {
		r.stashed = append(r.stashed, pid)
	}
	r.results[pid] = waitResult{status: status}

	if len(r.stashed) > maxStashedResults {
		delete(r.results, r.stashed[0])
		r.stashed = r.stashed[1:]
	}
}

// claim takes pid's stashed status, if one arrived already. Callers hold
// r.mu.
func (r *reaper) claim(pid int) (waitResult, bool) {
	res, ok := r.results[pid]
	if !ok {
		return waitResult{}, false
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

// Wait blocks until pid has been reaped and returns its exit status.
func (r *reaper) Wait(pid int) (int, error) {
	r.mu.Lock()
	if res, ok := r.claim(pid); ok {
		r.mu.Unlock()
		return res.status, nil
	}
	ch := make(chan waitResult, 1)
	r.waiters[pid] = ch
	r.mu.Unlock()

	res := <-ch
	return res.status, nil
}
