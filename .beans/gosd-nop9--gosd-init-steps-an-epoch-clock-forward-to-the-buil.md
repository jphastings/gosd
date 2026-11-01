---
# gosd-nop9
title: gosd-init steps an epoch clock forward to the build timestamp at boot
status: completed
type: feature
created_at: 2026-11-01T18:11:20Z
updated_at: 2026-11-01T18:11:20Z
---

Today an RTC-less board (the whole Pi family, and any RTC board whose
coin cell is flat) boots at the Unix epoch and stays there until SNTP
answers. `config.json.buildTimestamp` (gosd-0esw) is only ever a validity
gate on NTP results — gosd-jyq8's audit confirmed nothing applies it to the
clock, even though that bean's own brief asked for "only RAISE an
obviously-wrong clock". Meanwhile `docs/ingress.md` already claims the
baked timestamp "keeps TLS validity mostly correct even before NTP has
actually landed", which is false, and cloudflared/tsfunnel start after a
2-minute time-synced timeout on that assumption.

Step the clock forward to the build timestamp at boot, so TLS works before
NTP lands (while the image is fresher than the server's cert rotation).

## Locked decisions

- **Forward only.** If the system clock reads before `cfg.BuildTime()`,
  set it to `cfg.BuildTime()`; otherwise leave it alone — a later,
  HCTOSYS-seeded RTC time is never pulled back.
- **Early.** In gosd-init, before the app (and before timesync/ingress)
  starts, so nothing launched sees the epoch.
- **No floor, no step.** A zero `BuildTime()` (config.json baked before
  gosd-0esw) does nothing; timesync already logs that the floor is disabled.
- **Never written to the RTC.** Only an NTP-derived time is written back
  (`rtcWriteback`); a stale build time must not be persisted as if real.
- **`/run/gosd/time-synced` stays the only "synced" signal.** Stepping to
  the build time does NOT create it. Apps and gosd-init's own code
  (crash-report `clockSynced`, ingress waits) keep gating on the marker.
- **Log the step** on the console, e.g. "clock read <t>, before this
  build; stepped forward to the build time <floor> until NTP syncs".
- **Not in scope:** fake-hwclock-style persisting of a last-known time to
  `/data` (SD writes plus a crash-ordering argument). Revisit only if stale
  long-running images become a real problem.

## Accepted security cost (record in docs)

At the epoch every TLS validation fails closed. At the build time, a
certificate that expired *after* the build validates until NTP lands, and
an on-path attacker who blocks UDP 123 can hold the clock there. Go's x509
doesn't check revocation, so expiry is what bounds a leaked key; this pins
that bound at the build date. Exposure grows with image age. Accepted: the
alternative is no pre-NTP TLS at all.

## Todos

- [x] Forward-only boot step wired into gosd-init before anything else
      starts, following the package's seam pattern (pure logic + fakes,
      real syscall in `platform_linux.go`)
- [x] Behavioral tests: clock before floor → stepped to floor; clock after
      floor → untouched; zero floor → untouched; step never writes the RTC
      and never creates the time-synced marker
- [x] Docs: runtime.md "Clock" section (heading anchor is linked from
      ingress.md — update both), ingress.md's two clock passages,
      crash-reports.md, tsfunnel's timeout error text, any code comments
      that now say "starts at 1970"/"floor is never applied"
- [x] Change file (gosd-init behaviour change is user-facing)
- [x] Quality gates

## Summary of Changes

- `timesync.StepToFloor` (new, `floorstep.go`) sets the system clock to the
  build time through the existing `SystemClock` seam when it reads earlier,
  and logs the step. Later clock or zero floor: untouched. It writes
  neither the RTC nor `/run/gosd/time-synced`; `Run` still never applies the
  floor.
- `boot.Deps.StepClock` calls it from `boot.Run` straight after config.json
  is read, before networking, ingress or the app start; `main.go` wires it.
- Docs: runtime.md Clock section (heading renamed, ingress.md links
  updated; accepted security cost recorded), ingress.md, crash-reports.md,
  tsfunnel's timeout error, and code comments claiming a 1970 clock or an
  unapplied floor.
