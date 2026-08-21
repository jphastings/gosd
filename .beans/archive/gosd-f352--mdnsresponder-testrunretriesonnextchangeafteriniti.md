---
# gosd-f352
title: 'mdnsresponder: TestRunRetriesOnNextChangeAfterInitialFailure is timing-flaky on macOS CI runners'
status: completed
type: bug
priority: low
created_at: 2026-08-07T18:20:00Z
updated_at: 2026-08-20T05:55:04Z
---

Observed 2026-08-07: PR #198's test (macos-latest) job failed on this test
(0.25s in) in a package the PR does not touch (its diff is internal/container
+ docs only); a rerun was issued. The test exercises mdnsresponder.Run's
retry-on-next-Changed behavior, which involves the 250ms minRestartInterval —
on slow shared CI runners the timing margin is evidently too thin. Fix by
driving the loop with a fake clock or widening the tolerance so the test
asserts ordering (retry happened after a Changed) rather than wall-clock
timing. Check collision_test.go and server_test.go for the same pattern
while in there.

## Summary of Changes

**Root cause was not the 250ms minRestartInterval margin.** It was a
synchronization bug in the test itself: both TestRunStartsResponderOnceAtStartup
and TestRunRetriesOnNextChangeAfterInitialFailure polled ns.callCount()
reaching a target value, then immediately (no further synchronization)
checked a log line. But restart() calls deps.NewServer (bumping callCount)
and only writes the corresponding log line in a LATER statement — nothing
enforces happens-before between "callCount observed" and "the log line
exists". Under CI scheduler contention this gap is observable: the retry
test's second check happens right as the goroutine wakes from its
minRestartInterval timer (~0.25s in, matching PR #198's failure), which is
exactly the kind of wake-up point where a goroutine can be preempted before
completing its next couple of statements on an overloaded shared runner.

**Fix**: both tests now wait on the actual log line each assertion cares
about (waitFor(... log.contains(...) ...)) instead of polling callCount and
then checking the log unguarded; the callCount check moved to an ordinary
post-condition assertion after the log line is confirmed present. This
removes the race entirely rather than widening a timeout or adding a retry —
minRestartInterval and the 2s waitFor deadline are both untouched.
TestRunRestartsAndClosesPreviousResponderOnChange already only polled
values via waitFor with no unguarded follow-up check, so it needed no change.

Checked collision_test.go and server_test.go for the same pattern per this
bean's instruction: neither has it. collision_test.go's probeForCollision
calls are synchronous (no goroutine, no channel), and server_test.go's two
tests either poll nothing across goroutines or synchronize entirely through
blocking channel operations/real socket I/O — no proxy-then-unguarded-check
pattern in either file.

Verified with `go test ./cmd/gosd-init/internal/mdnsresponder/... -race
-count=10`: all tests, including both fixed ones, pass every iteration.
