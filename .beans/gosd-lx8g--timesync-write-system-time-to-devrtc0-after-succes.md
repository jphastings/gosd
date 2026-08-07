---
# gosd-lx8g
title: 'timesync: write system time to /dev/rtc0 after successful SNTP sync'
status: todo
type: task
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-07T12:53:05Z
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
