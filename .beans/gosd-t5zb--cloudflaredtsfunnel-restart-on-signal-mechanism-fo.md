---
# gosd-t5zb
title: 'cloudflared/tsfunnel: restart-on-signal mechanism for runtime network changes'
status: todo
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:28:29Z
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
