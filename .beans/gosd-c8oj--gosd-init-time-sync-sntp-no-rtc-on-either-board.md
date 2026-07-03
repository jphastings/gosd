---
# gosd-c8oj
title: gosd-init time sync (SNTP) — no RTC on either board
status: todo
type: task
created_at: 2026-07-02T21:03:54Z
updated_at: 2026-07-02T21:03:54Z
parent: gosd-ko20
blocked_by:
    - gosd-vtce
---

Neither board has a battery-backed RTC; the clock starts at epoch and TLS/x509 will fail until synced.

Locked: github.com/beevik/ntp. After network-up (watch /run/gosd/network-up), query `pool.ntp.org` (make the server list a config.json field with that default), settimeofday on success, then re-sync hourly with small adjustments. Retry with backoff until first success. Write /run/gosd/time-synced on first success and log the step change. Document for app authors: gate TLS calls on GOSD readiness (runtime docs task) — checking /run/gosd/time-synced or just retrying.

- [ ] Implementation + unit test for the retry/refresh state machine (clock ops behind an interface)
- [ ] Set the system timezone handling explicitly to UTC (no /etc/localtime in the initramfs; Go defaults to UTC — just verify and note)

## Acceptance
On hardware: date is correct within seconds of network-up; an https request from the example app succeeds after time-synced.
