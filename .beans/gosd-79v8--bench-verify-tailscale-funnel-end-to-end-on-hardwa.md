---
# gosd-79v8
title: Bench-verify tailscale-funnel end-to-end on hardware (real tailnet)
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:10:20Z
updated_at: 2026-08-07T15:10:24Z
parent: gosd-65uy
blocked_by:
    - gosd-o68e
    - gosd-u2gz
    - gosd-1cqa
---

Tailscale epic gosd-65uy bean 8, final (after TS-5 gosd-o68e, TS-6
gosd-u2gz, TS-7 gosd-1cqa). Bench session on the sdwire rig against a real
tailnet; record results per item IN THIS BEAN. Can share a rig day with
cloudflared's gosd-igk0.

## Checklist

[ ] End-to-end funnel serve on pi-zero-2w AND pi-zero-w (GOARM=6; note
    32-bit crypto throughput — tailscale/tailscale#7053)
[ ] FAT32 state-dir behavior: chmod/rename/logtail buffer files on vfat;
    power-cut during a state write. Designed fallback if broken: custom
    FAT-safe ipn.StateStore (WriteFileDurably semantics) via
    tsnet.Server.Store — a follow-up bean, not a redesign
[ ] Reboot + Imager-reflash persistence: same node, same URL, no re-auth
    (the layered property TS-6 asserts in tests, proven on hardware)
[ ] Missing funnel-nodeAttr UX: capture the exact upstream error text; feed
    the shim's wrapper wording and pin it in the shim's error-mapping test
[ ] Invalid + expired authkey behavior (clean error vs hang bounded by
    --register-timeout) → tighten TS-5's qemu CI assertion
[ ] Admin-console check: tags applied, node key expiry disabled
[ ] Serial log volume at 115200 acceptable (≤2 lines/min when permanently
    broken)
[ ] Measured shim size + RAM with the ts_omit set; boot-volume usage line
    (printBootVolumeUsage) co-baked with cloudflared
[ ] Cold boot with no network parks quietly; ACME/cert acquisition recovers
    after SNTP on a no-RTC Pi (clock-floor interaction)
[ ] Flip COMPATIBILITY.md "not yet hardware-verified" footnotes
