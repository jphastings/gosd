---
# gosd-vcnr
title: 'wifiup: backoff resets on CONNECT-accepted, not on confirmed association — wrong PSK reconnect-storms every ~3s forever'
status: todo
type: bug
priority: normal
created_at: 2026-07-31T07:53:06Z
updated_at: 2026-07-31T07:53:06Z
---

Found by review sweep `gosd-fuxs` (gosd-init runtime area), verified.

wifiup.go:217-243: `associate()` success → `backoff.Reset()` — but the
adjacent comment itself says the CONNECT ack only means the kernel
accepted the request; the 4-way handshake is still in flight.
`runUntilDisconnect` returns as soon as the 3s association poll sees
`Associated == false`, so the loop never grows its delay.

**Failure scenario:** wrong passphrase in gosd.toml — the single most
common misconfiguration: Disconnect → ConnectPSK (accepted) → Reset →
3s poll → not associated → repeat at a fixed ~3s cadence forever.
Continuous handshake attempts (some APs blacklist for this), serial log
spam, nl80211 churn; the backoff's stated purpose is defeated.

**Fix:** reset only once `Associated` has actually been observed true
(move Reset into watchAssociation or return it from runUntilDisconnect);
a cycle that never associated takes `backoff.Next()` like a failed
associate.
