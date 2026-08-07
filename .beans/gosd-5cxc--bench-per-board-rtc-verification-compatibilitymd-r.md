---
# gosd-5cxc
title: 'Bench: per-board RTC verification + COMPATIBILITY.md RTC rows'
status: todo
type: task
priority: normal
created_at: 2026-08-07T12:53:05Z
updated_at: 2026-08-07T12:53:22Z
parent: gosd-achn
blocked_by:
    - gosd-lx8g
---

RTC epic bean 3 (after the write-back bean). On the sdwire rig:

[ ] nanopi-zero2: /dev/rtc0 binds (HYM8563); with a coin cell on the 2-pin
    connector, time survives a full power cycle; without, survives warm reboot
[ ] rock-4se: same for the RK808 PMIC RTC (check battery connector presence)
[ ] radxa-zero-3e / cubie-a5e: characterize (PMIC/SoC RTC; battery pads?)
[ ] COMPATIBILITY.md: RTC rows per board + battery caveat (no battery =
    survives reboots, not power cuts); docs note in runtime.md's clock section
