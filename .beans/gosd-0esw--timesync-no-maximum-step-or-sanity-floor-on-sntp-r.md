---
# gosd-0esw
title: 'timesync: no maximum-step or sanity-floor on SNTP results — one spoofed UDP response sets the clock to any value'
status: todo
type: task
priority: normal
tags:
    - security
created_at: 2026-07-31T07:53:06Z
updated_at: 2026-07-31T07:53:06Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`applySync` (cmd/gosd-init/internal/timesync/timesync.go:194-201) applies
`newTime` unconditionally; beevik/ntp's `Validate()` checks only
stratum/leap/dispersion/freshness — all attacker-controlled in a forged
packet. Resync repeats hourly over unauthenticated UDP for the life of the
device.

**Failure scenario:** an on-path/LAN attacker races a forged response;
settimeofday lands 2019 or 2099. `/run/gosd/time-synced` already exists,
so apps gating TLS on it now validate certificates against a bogus clock —
accepting expired certs or failing closed — with no RTC, no interactive
surface, and no remote way to notice or fix it.

**Fix:** refuse any time before a build-baked floor (the build date is
known at image build); after first sync, cap the step (ntpd's classic
~1000s panic threshold) and log-and-refuse larger; optionally require
agreement from 2+ servers for the first step.
