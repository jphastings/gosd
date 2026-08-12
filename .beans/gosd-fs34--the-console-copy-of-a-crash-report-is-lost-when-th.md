---
# gosd-fs34
title: The console copy of a crash report is lost when the device halts
status: completed
type: bug
priority: normal
created_at: 2026-08-12T06:17:04Z
updated_at: 2026-08-12T07:38:03Z
parent: gosd-47z3
---

Found on the bench 2026-08-12, verifying `gosd-72ga`'s fix on a Pi 3B+.

`gosd-72ga` moved the console copy of a crash report from the app to
gosd-init, on the argument that gosd-init's version is strictly better —
it knows the device model, uptime and boot count the app cannot. The
UNRELEASED.md call-out states it plainly: *"gosd-init logs the complete,
real report to the serial console on its own once it commits one."*

**On the declared-fault path it doesn't.** The device halts before the
console write lands. A full 120-second capture of a real boot
(`serialcap.py`, raw termios) ends like this:

```
gosd/fault: HELLO-DEMO-FATAL — handed to gosd-init; see LAST_FATAL_ERROR.md on the boot partition; this device now stays down until someone power-cycles it
[gosd] /app (pid 156) exited with status 70 after 12.799479ms
[gosd] fatal: the app declared HELLO-DEMO-[    4.986641] reboot: System hal
```

`grep -c "error_code:"` over the whole capture finds nothing: no part of the
rendered report reached the console, and gosd-init's own final line is cut
mid-sentence by the halt.

The card copy is **complete and correct** — the write, fsync and remount all
finished. Only the console copy is lost, so this is a diagnostic-quality
issue, not a correctness or durability one.

## Why it matters anyway

It is a stated benefit of `gosd-72ga`'s chosen fix, and the reason that fix
was preferred over simply not folding the tail in. Someone with a serial
cable attached — the person most likely to be debugging — now gets *less*
than the card does, which is the opposite of what the change promised.

## Scope

Confirmed on the `fault.Fatal` (halt) path. **Not yet checked** on the two
other paths that end in a halt or a reboot: `haltForDataCorruption`, and the
generic `fatal()` reboot-after-5s path. The reboot path has a deliberate 5s
pause, so it may well be fine; the halt paths are the suspects. Check all
three before deciding the fix is complete.

## Fix direction (not locked)

Order and flushing, not content. Emit the console copy *before* initiating
the halt, and make sure the console write is flushed/synced rather than
racing `unix.Reboot`. `Rebooter.Sync()` already exists on the fatal paths for
disks — the console needs the equivalent guarantee, or simply needs to
happen earlier.

## Do this too, whichever way it goes

`docs/releases/UNRELEASED.md` currently makes the untrue claim, and it has
not shipped yet — v0.4.0 is out, this note is for the next tag. Either fix
the behaviour or soften the note before it becomes a released promise. Same
for anything in `docs/crash-reports.md` that repeats it.

## Verified working in the same run (do not re-litigate)

- `gosd-72ga`'s actual fix: the report no longer nests a copy of itself.
  1104 bytes on the Pi versus 2025 on the pre-fix NanoPi run, and the
  technical detail holds only the short pointer line.
- `device: Raspberry Pi 3 Model B Plus Rev 1.3 (pi-3b)` — the device-tree
  read on the Pi family, closing the last gap in epic `gosd-47z3`.

## Summary of Changes

Confirmed all three fatal paths and fixed the two that were actually broken:

- `haltForAppFault` (the declared-fault halt path — the one reproduced on
  the bench): broken. `Sync()` then `Halt()` immediately, no drain.
- `haltForDataCorruption`: broken the same way — it runs through `fatal()`'s
  halt branch, which also went straight from `Sync()` to `Halt()`.
- The generic `fatal()` reboot-after-5s path: not actually broken (the 5s
  `Sleep` before `Reboot()` gave the console line time to drain in
  practice), but that was incidental, not guaranteed — a big enough report
  or a slow enough baud rate could still lose the tail. Now it gets the same
  explicit guarantee as the halt paths rather than relying on timing.

**Fix:** added `FlushConsole()` to the `Rebooter` interface. The real
implementation (`platform_linux.go`) opens its own handle to `/dev/console`
and issues `TCSBRK` with a nonzero argument — glibc's `tcdrain(3)` on Linux —
which blocks until everything already written to the console has actually
been transmitted, rather than just queued in the kernel's tty buffer. It
doesn't need the same fd the logger wrote through: the output queue TCSBRK
drains belongs to the underlying tty device, not to any one file descriptor.
`deps.Rebooter.FlushConsole()` is now called right after `Sync()` and right
before every `Halt()`/`Reboot()` call in `fatal()` and `haltForAppFault`, and
— for consistency, since it's the same Sync-then-shutdown shape — in
`PanicGuard.Reboot` too (a panic's stack trace deserves the same guarantee).
`platform_other.go`'s non-Linux stub is a no-op, matching its other Rebooter
methods.

Content is unchanged, per the bean's direction: no code touches what gets
logged, only when the shutdown syscall is allowed to happen relative to it.
`fault/fault_test.go`'s `TestADeclaredFaultsCardCopyNeverContainsItsOwnBodyTwice`
(the gosd-72ga regression test) still passes untouched.

**Testing:** since the actual bug is a race against a syscall the test
suite can't execute (physical UART drain timing), the tests assert ordering
at the seam instead. `fakeRebooter` (fakes_test.go) now records the
sequence of methods called on it, and a new `assertBefore` helper checks
that `FlushConsole` precedes `Halt`/`Reboot` in that sequence — not merely
that both happened. This assertion was added to: the existing
`haltForAppFault` unit test (appfault_test.go) and its end-to-end `Run()`
counterpart, the data-corruption halt test, the boot-mount-timeout reboot
test via `assertFatalPathTriggered` (now shared by all reboot-path tests),
and `PanicGuard`'s own reboot test. Confirmed each of these five new
assertions actually fails without the fix (reverted the three source files,
reran — all five failed with "FlushConsole was never called" — then
restored).

**Docs:** `docs/releases/UNRELEASED.md`'s bench-tested caveat is replaced
with an accurate account: the gap it flagged is closed, every fatal path
now blocks until the console has actually drained before it halts or
reboots. `docs/crash-reports.md` made no caveated claim, so it needed no
change — its existing "gosd-init logs the complete report to the serial
console" statement was already phrased as a plain claim, and that claim is
now true without qualification.

Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .` (clean),
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` (both 0
issues).



## Bench-verified on hardware, 2026-08-12 (before merge)

The fix was checked on the same Pi 3B+ that exposed the bug, since the
failure is a race against a syscall no test can execute — the unit tests can
only assert ordering.

Same image shape as the original reproduction (`examples/hello` with
`HELLO_FATAL`, `--data-size expand`), built from this branch, captured with
`serialcap.py` across a full boot.

**Before:** the capture contained no part of the report — zero occurrences of
`error_code:` — and gosd-init's final line was cut mid-word by the halt.

**After:** the complete rendered report reaches the console, frontmatter and
all, and exactly ONCE:

```
error_code: HELLO-DEMO-FATAL
timestamp: unknown
clock: unsynced — timestamp is not trustworthy
uptime: 5s
boot: 1
device: Raspberry Pi 3 Model B Plus Rev 1.3 (pi-3b)
image: "hello fs34-bench #701a6209ea0e"
```

Counted occurrences across the whole capture: `error_code:` 1, `device:` 1,
`uptime:` 1, `image:` 1, `# hello crash report` 1. Exactly one of each
matters as much as their presence — it confirms `gosd-72ga`'s fix still
holds and the console copy has not reintroduced duplication by another
route.
