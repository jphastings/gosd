---
# gosd-c8oj
title: gosd-init time sync (SNTP) — no RTC on either board
status: in-progress
type: task
priority: normal
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-04T12:27:58Z
parent: gosd-ko20
blocked_by:
    - gosd-vtce
---

Neither board has a battery-backed RTC; the clock starts at epoch and TLS/x509 will fail until synced.

Locked: github.com/beevik/ntp. After network-up (watch /run/gosd/network-up), query `pool.ntp.org` (make the server list a config.json field with that default), settimeofday on success, then re-sync hourly with small adjustments. Retry with backoff until first success. Write /run/gosd/time-synced on first success and log the step change. Document for app authors: gate TLS calls on GOSD readiness (runtime docs task) — checking /run/gosd/time-synced or just retrying.

- [x] Implementation + unit test for the retry/refresh state machine (clock ops behind an interface)
- [x] Set the system timezone handling explicitly to UTC (no /etc/localtime in the initramfs; Go defaults to UTC — just verify and note)

  Verified against the Go 1.26 stdlib source (time/zoneinfo_unix.go, initLocal): when $TZ is unset and /etc/localtime can't be loaded, Go falls through to "Fall back to UTC" unconditionally — no code needed. gosd-init's initramfs has neither, so time.Now()/time.Local are UTC from boot without any explicit configuration.

## Acceptance
On hardware: date is correct within seconds of network-up; an https request from the example app succeeds after time-synced.

## Summary of Changes

Added `cmd/gosd-init/internal/timesync`, a new package that synchronizes gosd-init's system clock via SNTP, following the fake-driven, thin-interface style established by `boot`/`netup`/`wifiup`:

- `NTPClient` (real implementation wraps the locked `github.com/beevik/ntp` dependency's `ntp.Time`) and `SystemClock` (settimeofday(2)) sit behind interfaces in `interfaces.go`; the retry/refresh state machine (`timesync.go`) has no build tags and is unit-tested with fakes on macOS. `SystemClock`'s real, syscall-backed implementation lives in `platform_linux.go` (`linux` build tag); `platform_other.go` stubs it for non-Linux builds. `NTPClient`'s real implementation (`ntpclient.go`) needed no such split — it's a plain UDP round-trip, not a Linux-specific syscall — so it's constructed identically by both `NewPlatform` variants.
- `Run(deps, opts)`: polls `deps.NetworkUp` (backed by `/run/gosd/network-up`) with the fake-able `Clock` until it appears (no inotify — a plain poll loop is enough), then retries the configured NTP server list with jittered exponential backoff until the first success. On success it calls `SystemClock.Set`, logs the step change (old time -> new time), and writes `/run/gosd/time-synced` (`DefaultTimeSyncedPath`). It then re-queries every `ResyncEvery` (production: hourly) for as long as the process runs, applying and logging each successful adjustment; a failed hourly resync is logged and left to the next scheduled attempt rather than spinning its own backoff loop, since the next attempt is already only an hour away.
- `Backoff` and `Clock` are timesync's own copies (not imported from `netup`), matching how `boot` already keeps its own independent `Backoff` rather than sharing one across packages — timesync only shares the network-up marker *path* with netup, wired together in `main.go`, not any types.
- `internal/initcfg.Config` gained an optional `NTPServers []string` field (`json:"ntpServers,omitempty"`); omitted/absent (including every config.json baked before this field existed) falls back to `timesync.DefaultServers` (`["pool.ntp.org"]`), applied in `main.go`'s `ntpServers` helper rather than inside `ParseConfig`, keeping the config schema and its defaults decoupled.
- `main.go` wires `timesync.NewPlatform()` and `timesync.Run` into `boot.Deps.StartNetworking` as a third concurrent activity alongside `netup.Run` and `wifiup.Run` — it does not block or delay `/app`'s start, and does not depend on netup/wifiup completing (it discovers network-up by polling the same marker file they write).

### Design decisions

- **UTC verified, no code needed**: confirmed against the Go 1.26 stdlib source (`time/zoneinfo_unix.go`'s `initLocal`) that when `$TZ` is unset and `/etc/localtime` doesn't exist, Go falls back to UTC unconditionally. gosd-init's initramfs has neither, so `time.Now()`/`time.Local` are already UTC from boot with no explicit configuration — noted directly on this bean's checklist.
- **NTP server fallback within a round**: `queryServers` tries every configured server in order before backing off, so one flaky server doesn't cost an entire backoff cycle — not specified by the bean but a small, low-risk robustness addition consistent with `netup`'s general retry philosophy.
- **Steady-state resync has no independent backoff**: a failed hourly resync just waits for the next scheduled hour rather than running its own bounded retry loop; the bean only asked for "retry with backoff until first success," so extending backoff logic to steady-state resyncs would have been scope creep.
- Left a note on `gosd-mr2n` (Go developer quickstart + runtime documentation) flagging that its existing "Clock: starts at 1970 until SNTP lands" section in `docs/runtime.md` is now stale, with the specifics (marker path, config field, default server, gating guidance) that section should be rewritten with — mirroring how `gosd-vtce` left its own note there.

### Deviations / what remains for hardware

- The bean's on-hardware acceptance items (real date correct within seconds of network-up; an https request from the example app succeeding after time-synced) cannot be verified in this environment and are left unchecked in the `## Acceptance` section; both code todos are checked. Keeping this bean `in-progress` rather than `completed` for that reason, matching how `gosd-vtce` handled the same situation.
