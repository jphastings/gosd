---
# gosd-f352
title: 'mdnsresponder: TestRunRetriesOnNextChangeAfterInitialFailure is timing-flaky on macOS CI runners'
status: todo
type: bug
priority: low
created_at: 2026-08-07T18:20:00Z
updated_at: 2026-08-07T18:20:00Z
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
