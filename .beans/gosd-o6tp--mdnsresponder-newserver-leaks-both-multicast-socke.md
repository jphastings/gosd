---
# gosd-o6tp
title: 'mdnsresponder: NewServer leaks both multicast sockets on its expected-at-boot failure path — fd exhaustion in PID 1'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:53:30Z
updated_at: 2026-07-31T07:53:30Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

`NewServer` (cmd/gosd-init/internal/mdnsresponder/server.go:39-57) opens
pc4/pc6 and returns on `mdns.Server` error without closing either. pion
mdns v2.1.0's `Server` also doesn't close conns it was handed on its early
returns (and leaks two unicast sockets of its own — upstream issue; per
the no-third-party-PRs rule, record the patch in this bean if we want it
fixed upstream). `Run` retries `NewServer` on every `Changed`
notification, and its own comment says failure is expected at boot before
any interface is up.

**Failure scenario:** ~4 fds burned per failed attempt. A WiFi-only board
flapping association (no lease), or booting networkless and notifying
repeatedly, walks PID 1 toward the default 1024-fd rlimit — after which
DHCP sockets, netlink conns, and device nodes all fail to open and the
network never recovers.

**Fix:** close pc4/pc6 on the error return in NewServer (both nil-guarded).
Optionally rate-limit NewServer attempts. The pion-internal unicast leak:
document/patch here, do not PR upstream without JP.
