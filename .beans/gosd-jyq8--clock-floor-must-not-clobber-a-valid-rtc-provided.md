---
# gosd-jyq8
title: Clock floor must not clobber a valid RTC-provided time (HCTOSYS interaction)
status: todo
type: task
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-07T12:53:05Z
parent: gosd-achn
---

RTC epic bean 2. HCTOSYS sets system time from the RTC in-kernel BEFORE init
runs. Audit the build-timestamp clock floor in timesync/guard: it must only
RAISE an obviously-wrong clock (epoch/pre-build), never pull a later,
RTC-provided time backwards — and must behave when the RTC is wrong-but-
plausible. Behavioral tests for: RTC later than build timestamp (kept), RTC
earlier (floored), no RTC (floored). qemu-virt's host-seeded PL031 gives CI a
real HCTOSYS read path — add a smoke assertion if it drops in cleanly.
