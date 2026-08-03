---
# gosd-ftw7
title: 'Bench pass: hardware-verify the 2026-08 runtime hardening batch'
status: todo
type: task
priority: high
created_at: 2026-08-03T18:30:55Z
updated_at: 2026-08-03T18:30:55Z
---

PRs #150-#177 changed a large slice of gosd-init runtime behavior. All
of it is unit- and QEMU-tested, but per COMPATIBILITY.md's verification
tiers, ✅ outside the hardware-proven core means code-complete — this
bean is the bench session that upgrades the load-bearing ones, ideally
on the pi-3b (the only dual-interface board, which several fixes
specifically target):

- [ ] Dual-interface marker refcount (gosd-akk4): eth+wlan up, pull the
      cable → /run/gosd/network-up stays; replug → addresses correct
      (gosd-1lx7: exactly one address after a lease change)
- [ ] Wrong-PSK backoff (gosd-vcnr): delays visibly grow on serial, no
      ~3s storm
- [ ] Reflash upgrade end-to-end (gosd-lirl/ry3b/acdn): expand image,
      write data + hand-edit [env], reflash newer build via Imager →
      /data intact, hand-edit restored, wizard-skipped WiFi rejoins
- [ ] panic=10 (gosd-fkkr): force a panic (bench build) → reboot within
      ~15s, not a wedge
- [ ] SNTP floor (gosd-0esw): boot with clock at epoch → time lands
      sane; optionally a spoofed early reply is refused (log line)
- [ ] data_flush default (gosd-9m1k): eyeball write throughput vs a
      --data-flush build; durable-write counter still survives power cut
- [ ] Update COMPATIBILITY.md footnotes with bring-up-dated results in
      the same PR as any fixes this shakes out

Use the sdwire rig; file a bean per defect found rather than growing
this one.
