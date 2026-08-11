---
# gosd-s9uq
title: Capture the app's console tail and write it on a crash
status: todo
type: feature
priority: high
created_at: 2026-08-11T10:11:22Z
updated_at: 2026-08-11T14:41:09Z
parent: gosd-47z3
blocked_by:
    - gosd-pun9
    - gosd-m6py
---

Part of epic gosd-47z3. This is the layer that needs no cooperation from the
app at all, and the only one that can catch a panic, an OOM kill or a
segfault — which is to say, most real crashes. JP chose it explicitly
(2026-08-11) over "explicit reports only".

## The gap

`sequence.go` hands the app the raw console fd for both streams:

    return deps.AppStarter.Start(opts.AppPath, env, console, console)

So a Go panic's goroutine dump scrolls past on a serial cable nobody has
attached and PID 1 never holds a copy. `Supervisor.runOnce` logs the exit
status and restarts; nothing is recorded.

## Approach

Tee the app's stdout/stderr through a bounded ring buffer on the way to the
console — console output byte-for-byte unchanged, since that is the bench's
only diagnostic channel and must not regress. `internal/logwriter` already
does prefixed line-splitting for supervised children (cloudflared, tsfunnel)
and is the obvious neighbour, but this is a different job: no prefix, no
per-line logging, just retain the last N bytes. Decide whether to extend it
or add a sibling.

On a non-clean app exit, format the ring buffer's contents as the report's
"Technical detail" section and write `LAST_FATAL_ERROR.md` with
`error_code: GOSD-APP-CRASH`.

## Todos

- [x] Bounded ring buffer, default 64KiB, truncating from the FRONT with an
      explicit "(earlier output dropped)" marker — a Go panic prints every
      goroutine's stack, so the tail is where the useful part is, and an app
      that logs without newlines must never grow PID 1's memory (the concern
      `logwriter.MaxBufferedLine` already exists for). Landed standalone in
      `cmd/gosd-init/internal/consoletail` (PR #257) — pure buffer only, no
      wiring into sequence.go/supervisor.go yet; see the note below
- [ ] Tee to console unchanged; verify on the bench that serial output is
      identical to today
- [ ] Distinguish a crash from a clean exit. `Supervisor.runOnce` already
      has exit status and run duration. Locked: a non-zero exit status is a
      crash; exit 0 is not, even though the supervisor restarts it either
      way (an app that exits 0 on purpose is not broken)
- [ ] Signal deaths — SIGSEGV, SIGKILL from the OOM killer — must be named
      in human terms in "The problem" ("ran out of memory", not "signal 9").
      Check what `unix.WaitStatus` gives the reaper: `ExitStatus()` alone
      loses the signal, so `Supervisor.Wait`'s contract may need widening
- [ ] Honour the epic's write-rate rule: one report per stable-run cycle,
      not one per restart
- [ ] **Bound the re-arming, or this bean reintroduces the boot-FAT thrash
      the write-rate rule exists to prevent.** Found by gosd-pun9's
      adversarial pass: an app that dies just AFTER `StableRunThreshold`
      costs two remounts per cycle — delete the stale report on the stable
      run, write a new one on the crash — which at a ~35s cycle is roughly
      200 boot-partition remounts an hour. It is unreachable today because
      every existing fatal path reboots or halts, so nothing re-arms; this
      bean is what makes an app crash recoverable and therefore repeatable.
      The two locked rules ("one report per stable-run cycle" and "a
      recovered device stops looking broken") cannot both hold unbounded, so
      pick a bound — a cap per boot, a minimum interval between writes, or
      backoff on the re-arm — and record the reasoning. This is a
      crash-safety argument, not a tuning knob
- [ ] The "what it was doing" line has no app-supplied context on this path;
      fall back to "stopped unexpectedly while running"
- [ ] Fakes-driven tests that pass on macOS, per the gosd-init convention

## RESOLVED: secrets are redacted, not left to app discipline (JP, 2026-08-11)

The technical-detail section carries the app's console output verbatim, so
an app that logs a token puts it on the card — and the report invites the
owner to forward the whole file to a support site, which is where the
exposure stops being bounded. JP's answer is redaction rather than a
documented constraint or an off switch: gosd-m6py replaces env var values
with `{$ENV_VAR}` and app-registered secrets with `{secret: label}`, in the
renderer, so this path is covered by construction.

**gosd-m6py therefore BLOCKS this bean**, deliberately: the tail capture
must not ship before the thing that scrubs it.

Note the asymmetry that follows, and document it: the serial console keeps
receiving unredacted output. It is a physically-attached debug channel for
someone already holding the board, not a file that travels.

## Note: core landed separately from the wiring

The pure, standalone bounded-buffer package (`cmd/gosd-init/internal/consoletail`)
landed in its own PR against `main`, deliberately split from the wiring
(the remaining unchecked todos above: tee into `sequence.go`, crash-vs-clean-exit
detection in `supervisor.go`, signal-death naming, write-rate honouring, the
fallback narration line, and fakes-driven tests for all of that). The split
parallelises the work; the wiring is still blocked on gosd-pun9 and gosd-m6py
as noted above, so it lands in a second PR once those land. This bean's status
stays `todo` — it is not complete.
