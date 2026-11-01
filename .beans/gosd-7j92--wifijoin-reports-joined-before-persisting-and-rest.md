---
# gosd-7j92
title: wifi.Join reports Joined before persisting and restarting ingress
status: completed
type: bug
created_at: 2026-11-01T10:35:20Z
updated_at: 2026-11-01T10:35:20Z
---

`TestRuntimeJoinSameSSIDStillFiresIngressRestart` (cmd/gosd-init/internal/wifiup) fails intermittently on CI, and failed the release build after PR #394 and #395 merged:

    runtimejoin_test.go:342: ingress restarts after the same-SSID rejoin = 1, want 2 (epic decision 4)

## Root cause

`handleRequest`'s success callback (runtimejoin.go) writes the `Joined` status FIRST, and only afterwards runs the side effects that success implies: `deps.Persist` (when the request asked for it), then `deps.RestartIngress`. Both tests that assert those side effects right after observing `Joined` race them: this one, and `TestRuntimeJoinEndToEndThroughTheRealReconciler`'s persist and restart checks.

Proven deterministically: a temporary 100ms sleep right after the `Joined` write fails both tests every run.

It's a real product race too, not only a test one. `wifi.Join` returns as soon as it reads `Joined`. With `Options{Persist: true}` it can therefore return while `EditBootPartition` is still writing the network to the SD card, so an app that tells its user "saved, you can unplug" can lose the persisted network.

## Locked decisions

1. **Publish `Joined` last.** In the success callback, run persist, then `RestartIngress`, then write the `Joined` status. `RestartIngress` is a non-blocking `restartsignal.Notify`, so nothing is delayed. A persist failure is still logged and the status is still `Joined` (unchanged behaviour; whether `Join` should surface a persist failure is out of scope).
2. **State the guarantee on the public API.** `wifi.Options.Persist`'s doc says `Join` returns only once the credentials have been written.
3. **Add a regression test that pins the ordering:** a `Persist` fake that blocks keeps the status off `Joined` until it returns. The existing tests then pass deterministically, with no polling added to them.

## Todo

- [x] Reorder the success callback
- [x] Doc the guarantee on `wifi.Options.Persist`
- [x] Ordering regression test
- [x] Change file (user-facing: `wifi.Join` with `Persist` now returns only after the write)

## Summary of Changes

- `handleRequest`'s success callback now persists, restarts ingress, and only then writes the `Joined` status, so `wifi.Join` (which returns on reading `Joined`) can no longer return before the card write or ingress restart. Persist failures are still logged and the status is still `Joined`.
- `wifi.Options.Persist` documents the guarantee.
- `TestRuntimeJoinReportsJoinedOnlyAfterPersisting` pins the ordering with a blocking `Persist`; the previously flaky tests are now deterministic (still pass with a 100ms sleep before the `Joined` write).
- Patch change file for `gosd`.
- Also fixed the same log-before-status shape on the unparseable-request path (status written, then logged), which `TestRuntimeJoinUnparseableRequestSelfHeals` waits on via the log and which failed once in 200 runs.
