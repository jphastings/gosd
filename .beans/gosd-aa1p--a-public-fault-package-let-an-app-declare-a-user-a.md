---
# gosd-aa1p
title: 'A public fault package: let an app declare a user-actionable fatal error'
status: todo
type: feature
priority: high
created_at: 2026-08-11T10:11:29Z
updated_at: 2026-08-11T10:25:25Z
parent: gosd-47z3
blocked_by:
    - gosd-pun9
    - gosd-m6py
---

Part of epic gosd-47z3. The app-facing half: a semver-relevant exported
package alongside `gadget/`, `emmc/`, `disk/` and `sound/` that lets any GoSD
app declare a fatal, user-actionable condition and have it rendered into the
same `LAST_FATAL_ERROR.md` gosd-init writes for its own failures. JP's
framing (2026-08-11): "the external package we provide should format the
provided information in the same way", so the file is uniform whoever raised
it.

Where the crash-tail child catches what the app couldn't report, this catches
what the app understands and the tail never could: "your API key is wrong",
"that GPIO pin is already in use", "the config you edited names a sensor this
build doesn't support". Those have a real fix, and a report that states it is
the difference between a returned device and a two-minute edit on the card.

## LOCKED: the app never touches /boot

The app is uid 0 with `CAP_SYS_ADMIN` on an initramfs, so it *could* remount
`/boot` itself — and must not. It races gosd-init's own remounts, opens a
window where a live app has the boot partition writable, and cannot survive
the panic case regardless. The app writes a report to `/run` (tmpfs, already
mounted by `boot/mounts.go`) via write-to-`.tmp`-then-rename so gosd-init can
never read a half-written file, and gosd-init commits it to the card when the
app exits.

## Sketch

```go
package fault

// Report is a fatal condition described in terms its raiser understands
// and its reader doesn't.
type Report struct {
    Code    string // stable, greppable, app-defined: "NO-API-KEY"
    Doing   string // human: "fetching today's forecast"
    Problem string // human: "the weather service rejected our API key"
    Fix     string // human: "set WEATHER_API_KEY in gosd.toml on this card"
    Detail  error  // technical, verbatim, for whoever gets forwarded the file
}

// Fatal records r for gosd-init to write to LAST_FATAL_ERROR.md on the
// boot partition, then halts the device. It does not return, and the
// device stays down until someone power-cycles it: call it only for a
// condition no restart can improve.
func Fatal(r Report)

// RegisterSecretString ensures secret never appears in a crash report,
// replaced by {secret: replacement}. See gosd-m6py.
func RegisterSecretString(secret, replacement string)
```

## Todos

- [ ] Settle the package name. `fault` reads well at the call site
      (`fault.Fatal(...)`) and doesn't collide with stdlib `log`; alternatives
      considered: `crashreport`, `diag`, `report`
- [ ] `Fatal` is the whole reporting API — no non-exiting `Record(r)`: a
      "fatal error" the app survives is a contradiction, and with halt
      semantics locked there is nothing for it to return to
- [ ] `RegisterSecretString(secret, replacement string)` is exported from
      this package but specified and implemented in gosd-m6py, which blocks
      this bean. Keep the two in one package so an app has a single import
- [ ] Handoff format and path: `/run/gosd/fault.json`, write `.tmp` → rename.
      gosd-init reads and unlinks it in `Supervisor.runOnce` after the app
      exits. An unparseable or empty drop file is dropped, not trusted (the
      self-heal lesson from gosd-6cf2)
- [ ] Precedence when both a drop file and a crash tail exist for the same
      exit: the app's own explicit report wins the human sections; the tail
      still supplies "Technical detail" (a `fault.Fatal` call and a panic in
      another goroutine can genuinely coincide)
- [ ] Off-device degradation: on macOS, or under `go test`, or anywhere
      `/run/gosd` isn't there, `Fatal` renders the identical Markdown to
      stderr and exits — so a developer sees exactly what their user will,
      without flashing a card. This is the package's main development-time
      value and should be tested as behaviour, not an afterthought
- [ ] `platform_linux.go` / `platform_other.go` split per the convention, if
      it ends up needing any syscall at all
- [ ] The shared formatter lives in an internal package that both this and
      gosd-init import, so there is exactly one renderer (see gosd-pun9)
- [ ] Exported-API docstrings, a docs/ page, and an example. Consider whether
      `examples/hello` should raise one deliberately behind a flag so the
      flow is demonstrable end to end
- [ ] Add to CLAUDE.md's "Public API surface" bullet in the same PR

## LOCKED: fault.Fatal halts the device (JP, 2026-08-11)

"This mechanism is only for fatal errors that are not considered transient."
So there is no `Retry` field and no restart-by-default: an app calling
`fault.Fatal` is asserting that restarting cannot help, and restarting it
anyway would only spin the card and bury the report under a crash loop.
`fault.Fatal` records the report and the device halts.

That makes the package's doc comment load-bearing — an app author reaching
for it must understand it stops the device until someone power-cycles it.
Anything that might succeed on a retry should return an error and let the
supervisor's backoff do its job, and the docstring must say so in those
words.

**Scope of this decision:** it governs the DECLARED path only. The
crash-tail path (gosd-s9uq) fires on a panic or a non-zero exit that the app
never classified, where transience is unknowable — there the supervisor
keeps restarting with backoff exactly as it does today, and the report is
written alongside. Flagged for JP in case the intent was broader: making an
undeclared panic halt too would turn any transient app bug into a device
that stays down until someone visits it.
