---
# gosd-ojbm
title: 'Runtime WiFi join: app-supplied credentials via wifi.Join + automatic ingress reconnect'
status: todo
type: epic
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:28:29Z
---

JP request (2026-09-01, planning session): a GoSD app must be able to trigger joining a new WiFi access point at runtime, with credentials the app obtained by its own means (the motivating example: an NFC tag), and the ingress tunnel (cloudflared / tailscale-funnel) must come back on the new network without a reboot. Primary use case: the device boots in a new location where WiFi credentials aren't known — possibly with no WiFi configured at all.

## Locked decisions (JP, 2026-09-01 planning session)

1. **Transport: desired-state file under `/run/gosd/wifi/`, no socket.** Reuses gosd-qfbk decision 11's blessed idiom wholesale: the public package writes `request.json` atomically (temp + rename, file 0600, dir 0700); gosd-init polls and reconciles; the outcome comes back via `status.json`. No IPC socket, no listener — the "mDNS is the only network listener in gosd-init" locked decision is untouched. Bean gosd-rb4u remains the only place a socket is even under discussion. The request/status file schema and atomic-write helpers live in one shared `internal/wifictl` package so the two sides cannot drift.

2. **Public API: blocking `wifi.Join`.** New top-level public package `wifi/` (semver-relevant public API — CLAUDE.md's public-API bullet gains it in the docs child bean): `wifi.Join(ctx, wifi.Credentials{SSID, Passphrase}, wifi.Options{Persist: bool}) error`. Join writes the request with a unique id, polls `status.json` until that id reaches a terminal state, and returns nil on joined / an error carrying the failure reason as precisely as nl80211 reports it (a wrong WPA2 passphrase usually surfaces as a handshake timeout — the error text must be honest about that ambiguity). ctx cancels the *wait*, not the join attempt gosd-init is making. Off-device behavior follows `fault/`'s precedent exactly — the **`gosd` build-tag axis, NOT `linux`/`!linux`**: without the tag, Join returns an immediate, actionable "not running on a GoSD device" error.

3. **Persistence is per-call** (JP 2026-09-01): `Options.Persist`. true → after a *successful* join, gosd-init writes the values into the card's config tree (`config/wifi/ssid`, `config/wifi/passphrase`) via the same write path the cloud-init seed consumption uses, so the next boot rejoins and the /data configstore carries them across reflashes. false → in-memory only; next boot uses whatever the tree already had. Failed credentials are never persisted.

4. **Ingress reconnect is automatic, and only for app-triggered joins** (JP 2026-09-01). A successful runtime join signals the cloudflared/tsfunnel supervisors to terminate and restart their child through the existing network-up gate. No app-facing ingress API. Ordinary network blips keep today's park-and-self-heal behavior (the locked-decision comment on cloudflared's supervise loop) — this epic does not touch it. The signal is in-process (mdnsresponder.Signal-style), not app↔init IPC. Every successful runtime join fires it, even one landing on the same SSID. A deliberate restart resets the child's backoff and is not counted as a crash.

5. **The reconciler runs even when no credentials were configured at boot.** `wifiup.Run` today returns immediately when neither the config tree nor config.json holds credentials — that would break this epic's primary use case. Restructure so the request watcher always runs on a WiFi-capable board, with the association loop idle until credentials exist (from boot config or a runtime request).

6. **No fallback to the previous network on failure.** After a failed join, the new credentials stay current and wifiup's normal backoff retry continues (out-of-range networks come back); the app already got its failure status and can submit another request. Last-write-wins: a single `request.json`, replaced atomically; `status.json`'s id tells the app whose outcome it is reading.

7. **Secrets:** `wifi.Join` registers the passphrase via `fault.RegisterSecretString` before writing anything (the gosd-aa1p write-through redaction), and gosd-init adds a redaction rule for a runtime-supplied passphrase alongside boot/sequence.go's existing STA rule at the moment it reads a request.

8. **Scope:** WPA2-PSK and open networks only, matching the fleet WiFi scope decision; a 64-hex-char passphrase is a pre-derived PSK, same as `ConfigCredentials`. Out of scope for v1: AP↔STA orchestration (see below), WPA3/EAP, and restarting the app process on network change (apps own their own reconnects, per the runtime contract). A board without WiFi hardware fails the join with an honest "no WiFi interface" status — a runtime error, not a build refusal.

9. **Interaction with epic gosd-qfbk (WiFi AP mode):** independent epics, both using `/run/gosd/<feature>/`. gosd-qfbk decision 4 makes AP and STA mutually exclusive — when the AP is enabled, wifiap.Run runs *instead of* wifiup.Run, so no join reconciler would be listening and `wifi.Join` would hang until ctx timeout. **Whichever epic lands second must make Join fail fast ("interface is in AP mode — disable the AP first") rather than time out.** v1 does not auto-disable the AP.

## Child-bean order

`internal/wifictl` protocol + public `wifi/` package (gosd-sqtb) and the ingress restart mechanism (gosd-t5zb) are independent and can proceed in parallel → wifiup reconciler + persist + main.go wiring (gosd-uy4x, depends on both) → docs + CLAUDE.md amendment + COMPATIBILITY.md (gosd-kg73) → bench verification (gosd-8vwy, hardware).
