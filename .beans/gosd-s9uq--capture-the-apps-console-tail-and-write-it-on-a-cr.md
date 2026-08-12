---
# gosd-s9uq
title: Capture the app's console tail and write it on a crash
status: completed
type: feature
priority: high
created_at: 2026-08-11T10:11:22Z
updated_at: 2026-08-12T07:16:46Z
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
- [x] Tee to console unchanged; verify on the bench that serial output is
      identical to today. Wired (`sequence.go`'s `appOutput := io.MultiWriter(console, tail)`)
      and proven by an automated test asserting the app's stdout/stderr reach
      the fake console verbatim (`TestRunTeesAppOutputToConsoleUnchanged`) —
      MultiWriter passes each writer the identical byte slice, so ordering
      console before tail can't alter what the console receives. Left
      unchecked: nobody has plugged a serial cable into a real board running
      this yet, matching gosd-pun9's own outstanding bench todos
- [x] Distinguish a crash from a clean exit. `Supervisor.runOnce` already
      has exit status and run duration. Locked: a non-zero exit status is a
      crash; exit 0 is not, even though the supervisor restarts it either
      way (an app that exits 0 on purpose is not broken)
- [x] Signal deaths — SIGSEGV, SIGKILL from the OOM killer — must be named
      in human terms in "The problem" ("ran out of memory", not "signal 9").
      Check what `unix.WaitStatus` gives the reaper: `ExitStatus()` alone
      loses the signal, so `Supervisor.Wait`'s contract may need widening
- [x] Honour the epic's write-rate rule: one report per stable-run cycle,
      not one per restart
- [x] **Bound the re-arming, or this bean reintroduces the boot-FAT thrash
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
      crash-safety argument, not a tuning knob. **Chosen: a hard cap of 10
      report writes per boot** (`maxReportsPerBoot`, `report.go`), gating
      only new writes and not cleanup, so total remounts this boot are bounded
      at 2*cap+1 however long a crash loop runs, and a device that genuinely
      recovers after its last recorded crash still gets its stale report
      cleaned up exactly once. See `report.go`'s doc comment for the full
      argument and `report_test.go`'s
      TestFatalReporterBoundsTotalWritesPerBoot /
      TestFatalReporterCleansUpOnceAfterTheCapIsHit for the proof.
- [x] The "what it was doing" line has no app-supplied context on this path;
      fall back to "stopped unexpectedly while running" (`Doing: "running"`,
      composed by the renderer's fixed "Your device stopped while running."
      sentence; the "unexpectedly" framing lives in Problem instead — see
      `appcrash.go`)
- [x] Fakes-driven tests that pass on macOS, per the gosd-init convention

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

## Summary of Changes

Landed the wiring PR: [jphastings/gosd#261](https://github.com/jphastings/gosd/pull/261).
(#257 was the pure `consoletail` buffer; this is the second PR the bean's
own note said would follow, now that gosd-pun9 and gosd-m6py have both
merged.)

**Tee.** `sequence.go` now builds one `consoletail.Buffer` per boot and tees
`/app`'s stdout/stderr through `io.MultiWriter(console, tail)`, console
listed first so ordering can't change what reaches it (MultiWriter hands
each writer the identical slice regardless of position, and `Buffer.Write`
never errors or blocks per its own doc). Proven with
`TestRunTeesAppOutputToConsoleUnchanged`.

**Crash vs. clean exit, and signal detail.** `Supervisor.Wait`'s contract
widened from a bare `(status int, err error)` to `(ExitStatus, error)`
(`interfaces.go`), where `ExitStatus{ExitCode, Signaled, Signal}` carries
everything `unix.WaitStatus` knows — `ExitStatus()` alone returns -1 for
both a signal death and "hasn't exited," which loses exactly the
information a human-readable crash report needs. The *reaper* (shared with
cloudflared and tsfunnel supervision) now stores and returns the full
`ExitStatus`; `cmd/gosd-init/main.go` wires cloudflared/tsfunnel through a
new `exitCodeOnly` adapter that discards the new fields, so their own
logging is provably unchanged (`ExitCode` is exactly what `ExitStatus()`
already returned). `Supervisor` itself stays policy-free — a new `OnExit`
callback fires after every exit with the full `ExitStatus`; `isCrash` in the
new `appcrash.go` is what decides a crash (signaled, or non-zero exit code —
exit 0 is never a crash, even though the supervisor restarts either way).
Signal deaths are named in human terms (`crashProblem`/`signalDescription`):
SIGKILL reads as "ran out of memory" (the realistic cause with no shell to
send a manual kill -9), SIGSEGV/SIGABRT/SIGBUS/SIGFPE/SIGILL each get their
own plain-English line, anything else falls back to the signal's own name
and number. An unrecovered Go panic never reaches this table at all — the Go
runtime exits via `exit(2)`, not a signal, so it's covered by the bare
non-zero-exit-code branch.

**The re-arming bound.** `report.go`'s `fatalReporter` gains a third rule
alongside the two the epic already locked (one write per stable-run cycle;
delete on recovery): `maxReportsPerBoot` (10) caps how many reports
`record()` ever actually writes in one boot, regardless of how many
crash/recover cycles occur. It gates only new writes, not `markStableRun`'s
cleanup — cleanup stays keyed on `Exists()`, unconditional on the cap — so a
device that genuinely recovers after its last recorded crash still gets that
stale report deleted exactly once, and total fault-reporter remounts this
boot are bounded at `2*maxReportsPerBoot+1` however long the crash loop
runs: a hard ceiling on the total, not merely a lower rate that still
accumulates without bound over a long enough uptime. See `report.go`'s doc
comment for the full argument;
`TestFatalReporterBoundsTotalWritesPerBoot`/
`TestFatalReporterCleansUpOnceAfterTheCapIsHit` pin it.

**Redaction.** Not reimplemented — confirmed instead. `newAppCrashReport`
returns a plain `faultreport.Report` fed into the exact same
`fatalReporter.record` → `faultreport.Render` path every other fatal class
already uses, so the app's own env-value scrub and `/run` secret
registrations apply by construction.
`TestRunRedactsAnEnvSecretFromAnAppCrashReport` proves it end-to-end for
this path specifically.

**Docs.** `docs/crash-reports.md`'s status banner and "What you get for
free" section now describe the shipped behaviour (including the new
`GOSD-APP-CRASH` row and the three-rule write-rate list); `docs/runtime.md`
gets a one-line cross-reference from "Logging"; `docs/releases/UNRELEASED.md`
gets a release-notes callout.

### The re-arming bound, argued explicitly

Two locked rules — "one report per stable-run cycle" and "a recovered device
stops looking broken" — together only bound the *rate* of remounts during an
indefinite crash loop, not the *total* over an indefinitely long uptime (an
app crash, unlike every existing gosd-init fatal, never reboots or halts, so
nothing stops the loop on its own). `maxReportsPerBoot` closes that gap with
a hard ceiling scoped to one boot: at most 10 writes, ever, however long or
fast the loop runs, and the counter naturally resets on the next real
reboot. It only gates writes, not cleanup, specifically so hitting the cap
doesn't trade "remounts too often" for "looks permanently broken after a
real recovery" — the one trailing cleanup after the cap is what keeps that
promise. This is layered *underneath* the existing per-cycle `armed` gate,
not instead of it: `armed` still stops a same-cycle crash loop from writing
more than once per stable-run window; `maxReportsPerBoot` is what stops an
indefinitely long *sequence* of such windows from accumulating without
bound.

### Adversarial pass

- **Simulated the exact worst case the bean names**: an app that reliably
  dies right after `StableRunThreshold`, cycling `markStableRun()` then
  `record()` back to back, 50 times in `TestFatalReporterBoundsTotalWritesPerBoot`.
  Confirms the write count sticks at exactly `maxReportsPerBoot` rather than
  growing with the cycle count.
- **Checked the cap doesn't strand a real recovery**: after the cap is hit,
  20 further stable-run cycles with no more crashes still delete the last
  stale report exactly once and never remount again
  (`TestFatalReporterCleansUpOnceAfterTheCapIsHit`) — the device does end up
  looking recovered, it just can't be told about a crash that happens after
  its budget for this boot is spent.
- **Checked the reaper widening doesn't silently change cloudflared/tsfunnel
  behaviour**: `exitCodeOnly` extracts exactly the same `ExitCode` value
  `ExitStatus()` always produced, verified by reading through both packages'
  own `Wait` usage (logging only) — neither ever needed signal detail, so
  `main.go`'s adapter is the only file outside `boot` this bean touches for
  the widening.
- **Checked ordering in the tee can't leak or alter console bytes**:
  `io.MultiWriter` calls each writer with the identical slice in listed
  order and only stops early on that writer's own error; `consoletail.Buffer.Write`
  is documented to never error or block, so listing it after `console`
  can't change what console receives even under a pathological write.
- **Checked Go's own panic path is covered without a signal at all**: an
  unrecovered panic exits via `runtime.exit(2)`, not a signal — confirmed
  `crashProblem`'s non-signaled branch (bare exit code) is what actually
  fires for that case, not the signal table, so a plain Go panic isn't
  silently dropped through an untested gap between the two branches.
- **Confirmed the nil-reporter and nil-report paths stay safe**: `OnExit`
  calls `report.record(...)` unconditionally (no `report != nil` guard in
  `sequence.go`), relying on `fatalReporter`'s existing nil-receiver
  no-op contract — covered by the pre-existing
  `TestNewFatalReporterIsNilWithNowhereToWrite`, re-verified after this
  change (`go test` above).
- **One thing deliberately left for a human call**: `maxReportsPerBoot`'s
  value (10) is a judgement call, not derived from a formula — documented as
  such in `report.go` and in the bean, so JP can adjust it in review if 10
  reports' worth of history is more or less than useful in practice.

### What is NOT done, and why

- **Bench verification that serial output is byte-for-byte identical on
  real hardware** is still outstanding — nobody has plugged a cable into a
  board running this build. The relevant todo stays unchecked; the
  automated proof (`TestRunTeesAppOutputToConsoleUnchanged`) is the
  strongest claim this PR can honestly make without one.
- **The lane-warning seam for gosd-aa1p**: `newAppCrashReport`'s `Detail` is
  the tail and nothing else; it does not attempt to guess at or merge a
  future `/run/gosd/fault.json` drop file. That's deliberately left for
  gosd-aa1p to layer on top of `Supervisor.OnExit`/`report.record`, per the
  epic's own locked precedence ("the app's own explicit report wins the
  human sections; the tail still supplies Technical detail").



## Bench verification, 2026-08-12 — console fidelity PASSED

The outstanding todo was the one thing automated tests could not settle: that
teeing the app's streams through `consoletail` leaves the serial console
byte-for-byte unchanged. Serial is the bench's only diagnostic channel, so
"we did not break it" deserved real hardware.

Verified on a Pi 3B+ (the bench board; see gosd-pun9 for why it is a 3B+ and
not the 3B it was set up as). Method deliberately avoids diffing two boots,
since a fault present in both would hide: a throwaway app emitted a
DETERMINISTIC pattern, and the host reconstructed the exact expected bytes
independently and compared.

The pattern was chosen to attack the tee's known seams:
- 60 numbered lines per stream, so loss or reordering is visible
- a 2000-character line, proving nothing re-wraps or splits in transit
- multi-byte UTF-8 (`héllo — naïve ✓ 日本語`), proving nothing mangles non-ASCII
- a write with NO trailing newline, later completed by one — the case
  `consoletail`'s line-boundary handling could plausibly have disturbed

Result: **127 of 127 expected lines present, zero missing, zero corrupted,
order preserved on both streams.** Every byte the app wrote reached the
console intact.

One false alarm worth recording so nobody re-runs it: the first comparison
reported all 127 lines simultaneously missing AND unexpected, with the counts
matching exactly. That is the tty's `ONLCR` translation appending `\r` to
every line — the serial line discipline, not the app's bytes and not the
tee. Strip `\r` before comparing.

Evidence: the capture and the comparator live outside the repo (the app was a
throwaway, deliberately not committed). The claim in v0.4.0's release notes —
"your app's stdout/stderr still reach it byte-for-byte, exactly as before" —
is now backed by hardware rather than by unit tests alone.
