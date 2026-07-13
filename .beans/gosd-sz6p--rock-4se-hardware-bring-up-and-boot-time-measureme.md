---
# gosd-sz6p
title: 'ROCK 4SE: hardware bring-up and boot-time measurement'
status: todo
type: task
priority: normal
created_at: 2026-07-13T13:18:13Z
updated_at: 2026-07-13T13:26:10Z
parent: gosd-cuym
blocked_by:
    - gosd-h8a8
---

**First real-hardware bring-up of any GoSD board** — JP has the ROCK 4SE. Every COMPATIBILITY ✅ is code-complete-only today; expect this bean to shake out fleet-wide issues. Deviations become new beans, not inline fixes.

## Checklist

- [ ] Flash SD, capture full serial boot log (UART2 @ 1500000 baud) into this bean
- [ ] GbE: DHCP lease, mDNS resolution, HTTP reachable
- [ ] 5× power-cycle survival
- [ ] NVMe: /dev/nvme0n1 enumerates; exFAT mount via throwaway app (unix.Mount); read-throughput sanity — **use the actual betamin SSD** (RK3399 PCIe link-training quirks with some drives are a known risk)
- [ ] Header I2C visible via examples/i2cscan; GPIO via gpioinfo
- [ ] USB gadget mode reachable on the OTG port (existing serial/Ethernet functions)
- [ ] Boot-time baseline: power-on → /app exec via serial timestamps, recorded here as the baseline for a later dedicated boot-optimization bean
