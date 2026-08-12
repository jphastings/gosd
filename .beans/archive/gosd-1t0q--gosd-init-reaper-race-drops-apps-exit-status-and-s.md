---
# gosd-1t0q
title: 'gosd-init: reaper race drops /app''s exit status and Supervisor.Wait blocks forever — app never restarted'
status: completed
type: bug
priority: high
created_at: 2026-07-31T07:52:15Z
updated_at: 2026-07-31T07:52:15Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`Start` registers reaper interest only after the fork
(cmd/gosd-init/internal/boot/platform_linux.go: `cmd.Start()` then
`s.reaper.expect(pid)`), and `deliver` discards a reaped pid that is
neither awaited nor expected (platform_linux.go:192-209 — the "unrelated
grandchild" branch). `Wait` (::212-225) then parks on a channel that will
never be signalled, with no timeout.

**Failure scenario:** /app exits within microseconds of exec (bad baked
env, wrong-arch binary SIGILL, immediate os.Exit). The SIGCHLD drain reaps
it before `expect(pid)` runs; the status is discarded; `runOnce` blocks in
`Wait(pid)` forever. The device stays up — network, mDNS answering — with
no app and no further log after "started /app (pid N)", indefinitely.

The race itself is acknowledged in archived bean `gosd-kkz4` ("could in
theory still be reaped and discarded as an unrelated pid") but the recorded
consequence there is wrong-in-practice: it is not a lost log line, it is a
permanent supervisor hang.

**Fix:** have `deliver` stash every reaped pid's status in `results`
(bounded — prune beyond ~64 entries or by age) so `Wait` can claim an
early-arriving status regardless of `expect` ordering, making `expect` an
optimisation rather than a correctness requirement. Or give `Wait` a
timeout that errors so the supervise loop always progresses.

## Summary of Changes

Took the stash-every-pid route. `deliver` now stashes the exit status of
*every* reaped pid, so `Wait` can claim a status that arrived before anyone
asked about it — the ordering between the SIGCHLD drain and the supervisor's
`Wait` call no longer decides whether the status survives. With that,
`expect` had no job left: the `expected` map, the `expect` method and
`linuxAppStarter`'s reaper dependency are gone rather than kept as an
optimisation, which removes the wiring the race lived in. `Wait` keeps its
blocking-until-reaped behaviour; no timeout was added, because a timeout
would only convert a lost status into a wrong one (reporting "app exited"
while it is still running).

The stash is bounded at 64 entries, pruned oldest-first, so PID 1 doesn't
accumulate a map entry per orphaned grandchild for the life of the device.
Eviction can't lose the supervised app's status: gosd-init runs one child at
a time and calls `Wait` the moment `Start` returns, so an eviction that
mattered would need 64 *other* pids to be reaped inside that window — pinned
by a test that reaps a full stash minus one of grandchildren between the
app's exit and its `Wait`.

The pure bookkeeping (waiters, stash, `Wait`) moved to an untagged
`cmd/gosd-init/internal/boot/reaper.go`, leaving only the SIGCHLD signal
wiring and the `wait4(-1, WNOHANG)` drain in `platform_linux.go` — the shape
the rest of gosd-init's runtime code uses, and it means these tests run on
macOS as well as in CI. `reaper_test.go` covers: a status delivered before
`Wait` is still returned (this bean's failure — confirmed to hang the old
code), a status delivered while `Wait` is parked (unchanged behaviour), the
grandchild-burst case above, and the bound holding under long-running
reaping.
