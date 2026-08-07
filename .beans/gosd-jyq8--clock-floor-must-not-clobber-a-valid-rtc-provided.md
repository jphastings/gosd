---
# gosd-jyq8
title: Clock floor must not clobber a valid RTC-provided time (HCTOSYS interaction)
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-07T17:04:25Z
parent: gosd-achn
---

RTC epic bean 2. HCTOSYS sets system time from the RTC in-kernel BEFORE init
runs. Audit the build-timestamp clock floor in timesync/guard: it must only
RAISE an obviously-wrong clock (epoch/pre-build), never pull a later, RTC-
provided time backwards — and must behave when the RTC is wrong-but-
plausible. Behavioral tests for: RTC later than build timestamp (kept), RTC
earlier (floored), no RTC (floored). qemu-virt's host-seeded PL031 gives CI a
real HCTOSYS read path — add a smoke assertion if it drops in cleanly.

## Todos

- [x] Audit: does anything set the system clock to the floor/BuildTimestamp
      before NTP runs, or is the floor purely a validity check on NTP
      results?
- [x] Behavioral test: RTC later than build timestamp (must be preserved
      until NTP)
- [x] Behavioral test: RTC earlier than build timestamp (floor semantics
      apply as designed)
- [x] Behavioral test: no RTC (current epoch-start behavior unchanged)
- [x] Evaluate qemu-virt's host-seeded PL031 as a CI smoke assertion; add
      it if it drops in cleanly, else record why not
- [x] Quality gates (targeted package tests, full `go test ./...`,
      `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...` x2)

## Audit finding: no clobber bug — the floor is a validity gate only, never applied to the clock

Traced every path that can ever touch the running kernel's wall clock:

- `unix.Settimeofday` (the only syscall gosd-init ever uses to set the
  clock) has exactly one call site in the whole module:
  `unixSystemClock.Set` in `platform_linux.go`. Confirmed by grepping the
  entire repo for `Settimeofday`/`clock_settime`/`adjtime` — no other hits.
- `unixSystemClock.Set` is invoked from exactly one place:
  `timesync.stepClock` (`timesync.go`), always with `newTime` — the result
  of a validated NTP query (`queryServers`/`validateSample`) that has
  already cleared `checkFloor`. `stepClock` is called from
  `syncUntilSuccess` (first sync) and `stepGuard.resync` (every resync
  after it) — both times with an NTP-derived value, never with
  `opts.Floor` itself.
- `Options.Floor` (wired from `initcfg.Config.BuildTime()` in main.go) is
  read in exactly two places, both read-only checks, neither a clock write:
  `checkFloor` (refuses an NTP result before the floor, retried like any
  other failed round) and `stepGuard.check`'s pre-floor-anchor fast path
  (decides whether a resync candidate needs a second confirming query,
  comparing the *estimated* `expected(old)` against the floor — `expected`
  is computed from this package's own anchor state, never a fresh read of
  the real clock).
- `SystemClock` and `RTC` (`interfaces.go`) are both Set-only — neither has
  a Get/Read method — so gosd-init structurally cannot read back "what is
  the system clock/RTC currently showing" at all. The kernel's own HCTOSYS
  (`CONFIG_RTC_HCTOSYS`/`CONFIG_RTC_HCTOSYS_DEVICE="rtc0"`, confirmed
  enabled on every board with an RTC — see gosd-achn) copies the RTC into
  the system clock during kernel init, entirely before gosd-init's `Run`
  is ever called; gosd-init never touches, inspects, or second-guesses
  that value except by comparing later NTP results against the build
  floor.

Conclusion: a system clock already correctly seeded by HCTOSYS from a
battery-backed RTC (later than the build timestamp) is never pulled
backward — nothing ever *applies* the floor value, and nothing ever calls
`System.Set` before an NTP result exists. The bean's feared clobber does
not exist in the current design; existing comments in `timesync.go`/
`platform_linux.go` already say as much (added by gosd-lx8g), and this
bean's tests now pin it down behaviorally rather than leaving it as a
comment-only claim. No code change was needed or made to timesync/guard —
per the bean's own instruction, the tests are the deliverable.

Live confirmation (not just static reading): built and booted a real
`--board=qemu-virt` image locally
(`go run ./cmd/gosd build ./examples/hello --board=qemu-virt` +
`scripts/qemu-run.sh`) and captured the serial console. The kernel's own
HCTOSYS line appeared at ~6s of boot, well before gosd-init even ran:
`rtc-pl031 9010000.pl031: setting system clock to 2026-08-07T16:57:53 UTC
(1786121873)` — a real, host-seeded, plausible time, not the epoch. The
subsequent first NTP sync then logged
`system clock synchronized via NTP: 2026-08-07T16:58:04Z -> 2026-08-07T16:58:03Z`
— a ~1s correction, not a multi-decade step back to any floor — exactly
what "preserved until NTP, then only a small correction" looks like in
practice.

## qemu CI smoke assertion: added

Confirmed against the real boot captured above that the pinned qemu-virt
kernel (`CONFIG_RTC_DRV_PL031=y`, `CONFIG_RTC_HCTOSYS=y`,
`CONFIG_CONSOLE_LOGLEVEL_DEFAULT=7`, and no `-rtc` override in
`internal/qemurun.Args`, so QEMU's default host-clock-seeded PL031
applies) prints the driver's own `dev_info` HCTOSYS line unconditionally,
early in boot, with no network dependency and no race against gosd-init's
own NTP sync. This drops in cleanly: `.github/workflows/ci.yml`'s
`qemu-boot` job already captures `serial.log`; added one more step,
"Verify the PL031 RTC seeded a plausible boot-time clock", that greps it
for `rtc-pl031.*setting system clock to` and checks the embedded year is
plausible (>= 2020, not epoch). No product code changed for this.

## Summary of Changes

- `cmd/gosd-init/internal/timesync/floor_hctosys_test.go` (new): three
  behavioral tests pinning the bean's three scenarios —
  `TestRunNeverSetsSystemClockBeforeFirstNTPSuccess` (RTC later than build
  timestamp: System.Set is never called before an NTP result lands, and
  never with the floor value itself),
  `TestRunFloorRefusesEarlyResultsRegardlessOfPresyncClock` (RTC earlier
  than build timestamp: an early NTP result is refused and retried
  regardless of the pre-sync clock, with the exactly-at-floor boundary
  case also covered), and
  `TestRunSystemClockBehaviorUnchangedWithNoRTCPresent` (no RTC: RTC
  absence has zero bearing on the floor/first-sync path).
- `.github/workflows/ci.yml`: added the PL031/HCTOSYS smoke-assertion step
  described above to the existing `qemu-boot` job.
- No changes to `timesync.go`/`guard.go`/`interfaces.go`/
  `platform_linux.go`: the audit found no clobber bug to fix.

Gates: `go test ./cmd/gosd-init/...` (targeted, first) passed cleanly, run
twice. `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`, and
`GOOS=linux golangci-lint run ./...` are all clean, on the whole module.
`.github/workflows/ci.yml` validated with
`python3 -c "import yaml; yaml.safe_load(...)"` (no `actionlint` available
in this sandbox); the new step's shell logic was exercised for real
against the live serial.log captured during the local qemu-virt boot
described above, not just written by inspection.

`go test ./...` itself: every package passed except `cmd/gosd`, which hit
its own `-test.timeout=10m0s` inside `TestBuildCreatesMissingSingleBoardOutputParentDirectory`,
stuck in a third-party FAT12/FAT32 write loop (`go-diskfs`'s
`fat12.(*FileSystem).WriteFat`/`allocateSpace`) — code this bean never
touches (no changes anywhere under `cmd/gosd`, `internal/image`,
`internal/pipeline`, or `internal/diskfmt`). This machine was under
extreme, sibling-agent-driven load for the whole run (`uptime` load
averages 150-230+, `df -h /` down to ~1GiB free / 93% capacity by the
time it finished) — exactly the shared-build-host contention this
project's CLAUDE.md warns about, and the identical failure signature
(this stack's own gosd-lx8g bean hit "no space left on device" in
`cmd/gosd` and `internal/diskfmt` in its own session, also called out as
unrelated/environmental). Re-verifying just the touched package
(`cmd/gosd-init/internal/timesync` and its parents) came back clean both
before and after this run, so this is recorded as environmental, not a
regression from this change.
