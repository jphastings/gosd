---
# gosd-lx8g
title: 'timesync: write system time to /dev/rtc0 after successful SNTP sync'
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-07T16:43:23Z
parent: gosd-achn
---

RTC epic bean 1. Locked decisions:

- Explicit ioctl(RTC_SET_TIME) on /dev/rtc0 behind timesync's platform seam
  after each successful SNTP set — deterministic and fake-testable; do NOT
  rely on kernel SYSTOHC's sync-flag path (settimeofday never marks the clock
  synchronized, so it never fires).
- /dev/rtc0 absent (Pi boards) → silent skip, no log noise. Write failure →
  one logged warning, never fatal, doesn't affect the sync loop.
- Update the stale "neither board has a battery-backed RTC" comments in
  timesync.go, guard.go, interfaces.go, platform_linux.go, initcfg/config.go.
- Feature-module rules apply: fake-driven tests pass on macOS; real ioctl in
  platform_linux.go only.



## Summary of Changes

Implemented behind timesync's existing platform seam, threaded through the
same first-sync/resync call sites PR #202 (gosd-dqps) added:

- `interfaces.go`: new `RTC` interface (`Set(t time.Time) error`) and
  `ErrRTCNotPresent` sentinel — an implementation reports it on a board with
  no RTC at all so `rtcWriteback.apply` can tell "nothing to do here" apart
  from "the write actually failed"; updated the stale `SystemClock` comment.
- `rtc.go` (new): `rtcWriteback`, a small per-Run-call latch — `apply` writes
  the just-applied time to `deps.RTC`, treats `ErrRTCNotPresent` as
  permanently silent, and logs any other error exactly once per Run call
  (mirrors the existing "floor is disabled" one-shot boot line).
- `timesync.go`: `Deps.RTC` field; `Run` creates one `rtcWriteback` alongside
  `stepGuard` and threads it through `syncUntilSuccess`/`stepGuard.resync`
  into `stepClock`, which now calls `rtc.apply` only after a successful
  `System.Set` — a failed clock set never attempts an RTC write. Updated the
  stale package-doc "neither board has a battery-backed RTC" claim.
- `guard.go`: updated the stale comment in `stepGuard`'s doc explaining why
  the first sync skips step-guarding.
- `platform.go`: added `RTC` to the `Platform` bundle.
- `platform_linux.go`: `unixRTC` — ioctl(RTC_SET_TIME) via
  `unix.IoctlSetRTCTime`/`unix.RTCTime` (golang.org/x/sys/unix already wraps
  the ioctl and wire struct) on `/dev/rtc0`; presence is checked once in
  `newUnixRTC` (called once from `NewPlatform`, itself called once per
  gosd-init process), never re-probed per sync, so an absent RTC produces no
  per-sync log noise. Updated the stale `unixSystemClock` comment.
- `platform_other.go`: `unsupportedRTC`, mirroring `unsupportedSystemClock`,
  for non-Linux builds.
- `cmd/gosd-init/main.go`: wires `platform.RTC` into `timesyncDeps`.
- `internal/initcfg/config.go`: updated the stale `BuildTimestamp` comment
  (comment-only).
- `fakes_test.go`/`timesync_test.go`: new `fakeRTC`; `newTestDeps` now wires
  a default always-succeeding one so every existing test keeps passing
  unchanged.
- `rtc_test.go` (new): behavioral tests — RTC written after the first sync
  and after a confirmed large step but not for the refused intermediate
  candidate; not written when the NTP round fails or when `System.Set`
  fails; `ErrRTCNotPresent` produces zero RTC-related log lines; a
  persistently failing write logs exactly once across multiple syncs.

Gates: `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run
./...`, and `GOOS=linux golangci-lint run ./...` all clean for every package
this change touches. Also cross-compiled clean for `GOOS=linux
GOARCH=arm64` and `GOOS=linux GOARCH=arm GOARM=6`.

Surprise: this session's `go test ./...` hit `no space left on device` in
`cmd/gosd` and `internal/diskfmt` (unrelated packages, not touched by this
bean) — the shared build host's disk was exhausted by concurrent sibling
agents' image/kernel-build tests, confirmed by `df -h` showing 100%
capacity and by plain `go build`'s own linker failing with the same error.
Not a regression from this change: `cmd/gosd-init/internal/timesync` and
`internal/initcfg` both passed cleanly.
