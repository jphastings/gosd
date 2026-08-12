---
# gosd-72ga
title: A declared fault's report embeds a nested copy of itself
status: todo
type: bug
priority: high
created_at: 2026-08-12T03:43:35Z
updated_at: 2026-08-12T03:43:35Z
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
