---
# gosd-t5zb
title: 'cloudflared/tsfunnel: restart-on-signal mechanism for runtime network changes'
status: completed
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:55:02Z
parent: gosd-ojbm
---

Part of the runtime-WiFi-join epic — read the epic's locked decisions first; decision 4 governs this bean.

Give the cloudflared and tsfunnel supervisors a way to be told "restart your child now" from elsewhere in gosd-init. The producer (the wifiup reconciler) is a sibling bean; this bean adds the mechanism + tests only, and does NOT touch cmd/gosd-init/main.go (the sibling wires it so no dead wiring ships alone).

## Shape

- A coalescing restart signal in each package's `Deps` (mdnsresponder.Signal is the in-process precedent — a buffered(1) channel with Notify()/C(); reuse or mirror it, don't invent a new shape). Nil/absent signal = today's behavior exactly, so existing tests and main.go stay valid until the sibling bean wires it.
- While a child is running, a fire of the signal terminates it (SIGTERM via the existing platform seam). The supervise loop then proceeds as it already does on child exit: re-check the network-up marker (parking if the network is down) and start again.
- **A deliberate restart resets the backoff and is not counted as a crash** (epic decision 4) — the child was healthy; restarting into a fresh network must not inherit a crash-loop delay.
- A signal that fires while the supervisor is parked waiting for network-up, or between children, coalesces harmlessly (the next child start already picks up the new network) — no queueing, no double restart.
- cloudflared.go and tsfunnel.go are near line-for-line twins sharing childbackoff/logwriter; keep them twins. If the select/kill plumbing is identical, prefer extracting it into the shared package over copy-pasting a third variant — but do not refactor the supervisors beyond what this feature needs.

## Traps (both bitten before — read the comments at the named sites)

- NEVER `cmd.Wait()` directly — PID 1's SIGCHLD reaper owns waiting; see the comment near cloudflared.go:108 and use the existing `deps.Wait(pid)` seam.
- The supervise loop's park-not-backoff behavior while the network is down is a locked decision (comment at cloudflared.go:284) — the restart signal must not change it.

## Tests

Fake-driven, macOS-passing, behavioral: signal during a running child → child terminated, restarted after network-up, backoff reset; signal while parked → coalesced, no extra restart; nil signal → unchanged behavior. Concise.

## Notes

- Branch `bean/gosd-t5zb-ingress-restart-signal` from main. "Part of gosd-ojbm" in the PR body. Internal-only mechanism with no user-visible behavior change until wired — use the `no release notes` label rather than a change file (the sibling wiring bean carries the release note).
- Run every quality gate in CLAUDE.md (both golangci-lint invocations) before pushing; foreground `gh pr checks <n> --watch --interval 30` after.
- Do not merge the PR — JP reviews and merges.


## Summary of Changes

Added a coalescing restart signal to cloudflared's and tsfunnel's supervise loops (epic gosd-ojbm decision 4's mechanism, not its wiring):

- New shared package `cmd/gosd-init/internal/restartsignal`: `Signal` (mirrors mdnsresponder.Signal's buffered(1) Notify()/C() shape), `Signal.Drain()` (nil-safe; discards a pending notification without acting on it), and `WaitOrKill(sig, wait, kill)` — the identical select/kill plumbing both supervise loops needed, extracted once rather than forked a third time (mirrors the childbackoff/logwriter precedent, bean gosd-wxjy). A nil `*Signal` makes WaitOrKill exactly `wait()`, so unset RestartSignal reproduces today's behavior exactly.
- `cloudflared.Deps` and `tsfunnel.Deps` each gained `Kill func(pid int) error` and `RestartSignal *restartsignal.Signal` (both nil by default). `platform.go` in each package gained the real `Kill` (SIGTERM via `os.FindProcess`/`Process.Signal`, no "linux" build tag needed, same as StartProcess).
- `runOnce` in both packages now waits via `restartsignal.WaitOrKill`: a signal fired while the child is running terminates it (never via `cmd.Wait` — still goes through `deps.Wait`/the PID-1 reaper) and marks the exit as a deliberate restart, which resets the backoff regardless of run duration (decision 4: not a crash).
- `supervise` in both packages now calls `deps.RestartSignal.Drain()` right after `waitForNetworkUp` succeeds and before starting the next child, discarding any signal that fired while parked or during the backoff sleep between children — the coalescing behavior the bean specifies, with no queueing and no double restart. The park-not-backoff behavior while the network is down is untouched.
- `cmd/gosd-init/main.go` is untouched, as specified — Kill/RestartSignal are not wired into `cloudflaredDeps`/`tsfunnelDeps` there; the sibling bean does that.

Tests (fake-driven, behavioral, pass on macOS including under `-race`): per-package `TestSuperviseRestartSignalTerminatesRunningChildAndResetsBackoff` (signal during a running child → Kill called on the running pid → restart happens at the reset/base backoff delay, not the escalated one) and `TestSuperviseRestartSignalWhileParkedCoalescesHarmlessly` (signal fires while parked on network-up → the child that eventually starts is never killed). "nil signal = unchanged behavior" is covered by every pre-existing supervise/Run test continuing to pass unmodified, since none of them set RestartSignal. Also added real-process `TestKillTerminatesARealProcess`/`TestKillReturnsErrorForNoSuchProcess` (platform_test.go, no fakes) and restartsignal's own unit tests (coalescing, Drain, nil-signal/no-fire/fire behavior of WaitOrKill).

Quality gates all green: `go test ./...`, `go vet ./...`, `gofmt -l .` (empty), `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` (0 issues each). No change file — internal-only mechanism with no user-visible behavior change until the sibling bean wires it; PR should get the `no release notes` label.
