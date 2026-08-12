---
# gosd-72ga
title: A declared fault's report embeds a nested copy of itself
status: completed
type: bug
priority: high
created_at: 2026-08-12T03:43:35Z
updated_at: 2026-08-12T04:34:03Z
parent: gosd-47z3
---

Found on the bench 2026-08-12, first real-hardware run of the completed epic
`gosd-47z3` (nanopi-zero2, `examples/hello` with `HELLO_FATAL` set, image
built from v0.4.0 + `--data-size expand --console-baud 115200
--support-url ... --app-version 0.4.0-bench1`).

**A `fault.Fatal` report contains a complete, nested copy of itself.** The
file the device wrote is 2025 bytes, of which roughly half is a duplicate —
and the duplicate is the *thinner* off-device rendering, so it contradicts
the real header directly:

```
## Technical detail

    HELLO_FATAL="{$HELLO_FATAL}"

    console output up to the exit:

    ---
    error_code: HELLO-DEMO-FATAL
    timestamp: unknown
    clock: unsynced — timestamp is not trustworthy
    uptime: unknown          <- the real header says 3s
    boot: unknown            <- the real header says 1
    device: unknown          <- the real header says FriendlyElec NanoPi Zero2
    image: unknown           <- the real header says hello 0.4.0-bench1 #51199…
    ---

    # Crash report
    ... the entire body again, verbatim ...
```

## Why it happens

`fault.Fatal` does two things on-device: hands the report to gosd-init via
the `/run` drop file, AND renders it to stderr. `consoletail` is capturing
stderr, so the rendered report lands in the tail. gosd-init then applies
`gosd-aa1p`'s precedence rule — "the app's explicit report wins the human
sections; the tail still supplies technical detail" — and folds the tail,
which by now *is* the report, into the report.

Everything is behaving as specified. The rule is what's wrong: it assumes the
tail and the declared report are independent sources, and on the declared
path the tail's last output is the report itself.

Only hardware surfaced it. The unit tests exercise the drop file and the tail
separately; the acceptance test pins that a declared fault wins the human
sections, which it does. Nothing asserted the *absence* of a second copy.

## Who this hurts

The non-technical device owner, who is the entire audience for this file.
They open it and find the same report twice, the second time saying it
doesn't know what device it is.

## Fix direction (not locked — decide in the bean)

The obvious options, roughly in order of preference:

- **Don't fold the tail in at all when a drop file is present.** Simplest,
  and the declared report already carries the app's own `Detail`. Costs: a
  panic in another goroutine racing a `fault.Fatal` would lose its stack,
  which is the case the precedence rule was written for.
- **Suppress the on-device stderr rendering**, keeping it only off-device
  where it is the whole point. Costs the serial console its copy, which is a
  real loss for someone with a cable attached.
- **Strip the echo from the tail** before folding — fragile string surgery on
  the renderer's own output, and it would silently rot.

Whatever is chosen, the regression test is an assertion about the *whole
file*: the rendered report must not contain its own body twice. That is the
assertion nothing currently makes.

## Also worth fixing while here

`uptime: unknown / boot: unknown / device: unknown` in the off-device
rendering is honest but reads as broken. Consider omitting those lines
entirely off-device rather than printing them as `unknown`, so the developer
preview looks like a real report rather than a half-populated one.

## Verified working in the same run (do not re-litigate)

- `device:` read from `/sys/firmware/devicetree/base/model` — the gap named
  in v0.4.0's release notes, now closed for the Rockchip family. Reported
  `FriendlyElec NanoPi Zero2 (nanopi-zero2)`.
- Boot counter on `/data` (`boot: 1`), uptime, honest unsynced-clock handling.
- Env-value redaction on device: `HELLO_FATAL="{$HELLO_FATAL}"`.
- `fault.Fatal` halting the board, and the remount → write → fsync → remount
  path completing against a live vfat boot partition.

## Decision (implemented)

Went with the bean's recommended direction (validated, not the three original
options — see below): split what `fault.Fatal` prints by whether `/run/gosd`
is present.

- **On-device (handed to gosd-init):** prints one short line to stderr — the
  error code and a pointer, e.g. `gosd/fault: HELLO-DEMO-FATAL — handed to
  gosd-init; see LAST_FATAL_ERROR.md on the boot partition; this device now
  stays down until someone power-cycles it` — never the rendered report.
- **Off-device:** unchanged (still the full Markdown to stderr), plus the
  "worth fixing while here" item: a header field the preview can never
  honestly know (`uptime`, `boot`, `device`) is now omitted from the
  frontmatter entirely rather than printed as `unknown` (`faultreport.Context.Preview`).
- **gosd-init now logs the complete rendered report to its own console**
  every time `fatalReporter.record` commits one (`cmd/gosd-init/internal/boot/report.go`).

Confirmed the load-bearing assumption before relying on it: gosd-init's own
`log()` calls write directly to the console writer opened in `sequence.go`
(`console`/`w`, wrapped by `Logger.Printf`); `consoletail`'s `tail` is fed
*only* through `appOutput := io.MultiWriter(console, tail)`, which is passed
solely as the app subprocess's Stdout/Stderr to `AppStarter.Start`. gosd-init's
own log lines never flow through `appOutput`, so they structurally cannot
reach `tail` — proven both by reading `sequence.go` and by a new test,
`TestGosdInitsOwnConsoleLinesNeverReachTheCardReport`, which runs the real
`boot.Run()` and asserts the card report never contains a `"[gosd] "`-prefixed
line.

Rejected the bean's other two options: "don't fold the tail in at all" would
silently drop a genuinely coinciding panic's stack trace on another goroutine
(the exact case the fold rule was written for); "strip the echo from the
tail" is fragile string surgery on the renderer's own output that would
silently rot. The chosen fix keeps the serial console fully informative (a
strictly *better* copy than before, since gosd-init knows the device model,
uptime and boot count) and preserves the tail's real purpose.

## Regression test

`fault/fault_test.go`'s `TestADeclaredFaultsCardCopyNeverContainsItsOwnBodyTwice`
is the assertion nothing previously made: it calls the real `reporter.deliver`
on-device, captures exactly what a real device's consoletail would have
captured from this process's own stdout/stderr, parses the real drop file
(`faultdrop.Parse`), folds the two together with the real
`faultreport.FoldConsoleTail` (moved out of `boot` into `internal/faultreport`
so both packages share one implementation), and renders with the real
`faultreport.Render` — the same functions gosd-init's own `haltForAppFault`
calls. It asserts the final markdown has exactly one `"## Technical detail"`
section, exactly one `error_code:` frontmatter line, and no `"device: unknown"`
despite the real header knowing the device. Verified this actually catches
the bug: temporarily reverted `fault.go`'s fix and confirmed this test (and
the enhanced `TestReportsHandedOverAreWaitingForGosdInit`) fail with the exact
nested-copy shape from the bench evidence above, then restored the fix and
re-confirmed green.

Secondary coverage: `TestGosdInitsOwnConsoleLinesNeverReachTheCardReport`
(`sequence_test.go`, full `boot.Run()`) and
`TestFatalReporterLogsTheFullReportToTheConsole` (`report_test.go`) pin the
"gosd-init logs the full report, and that's safe because tail can't see it"
half of the fix.

## Verification note

`go test ./...` (foreground, default GOCACHE) passed every package except
`TestBuildIdentityUnaffectedByLabelPrefix` (cmd/gosd, --label-prefix vs build
identity — completely unrelated to this change, confirmed via `git diff --stat`
against main touching nothing in that area). Re-ran it alone with an isolated
GOCACHE and it passed in 40s: shared build-cache contention from JP's
concurrent bench session (per CLAUDE.md's documented flake pattern), not a
real failure. A follow-up isolated-GOCACHE full run then hit genuine disk
exhaustion ("no space left on device" on every failure, across packages this
change never touches — internal/build, internal/diskfmt, internal/image,
cmd/gosd's cross-compiles) from the machine's shared disk being tight under
concurrent bench work; cleaned up the isolated cache immediately. All
packages this bean actually touches (fault, internal/faultreport,
cmd/gosd-init and its boot subpackage) passed cleanly in every run, isolated
cache included.

## Summary of Changes

- `fault/fault.go`: on a device, `Fatal`/`deliver` now prints only a short
  pointer line to stderr on a successful handoff (never the rendered report);
  off-device behaviour is unchanged. `context()` marks `Preview: true` for the
  off-device render.
- `internal/faultreport/faultreport.go`: added `Context.Preview`, which makes
  `uptime`/`boot`/`device` omit their line entirely (rather than print
  `unknown`) in a preview render; exported `UnspecifiedCode`; moved
  `withConsoleTail` here from `boot` as `FoldConsoleTail`, the one shared
  fold implementation both `fault`'s regression test and `boot` now call.
- `cmd/gosd-init/internal/boot/report.go`: `fatalReporter.record` now logs the
  complete rendered report to the console every time it commits one.
- `cmd/gosd-init/internal/boot/appfault.go`, `sequence.go`: updated to call
  `faultreport.FoldConsoleTail` instead of the now-removed local
  `withConsoleTail`.
- Tests: new regression test in `fault/fault_test.go`
  (`TestADeclaredFaultsCardCopyNeverContainsItsOwnBodyTwice`) plus supporting
  assertions there and in `internal/faultreport/faultreport_test.go`,
  `cmd/gosd-init/internal/boot/report_test.go` and `sequence_test.go` — see
  the Decision/Regression test notes above.
- `docs/crash-reports.md`: documents the short on-device pointer, gosd-init's
  own console echo, and the off-device preview's field omission.
- `docs/releases/UNRELEASED.md`: call-out under "Other call-outs".
