---
# gosd-uy4x
title: 'wifiup: runtime join reconciler, per-call persist, main.go wiring'
status: todo
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T15:28:29Z
parent: gosd-ojbm
blocked_by:
    - gosd-sqtb
    - gosd-t5zb
---

Part of the runtime-WiFi-join epic — read the epic's locked decisions first; decisions 3, 4, 5, 6, 7 and 9 govern this bean. Depends on the protocol bean (internal/wifictl) and the ingress-restart-signal bean — both must be on main before this branches.

Make gosd-init act on runtime join requests: watch `/run/gosd/wifi/request.json`, reconcile, report status, persist on request, fire the ingress restart.

## wifiup reconciler

- A watcher in `wifiup` polls for a new request id on the marker-file cadence (2–3s, matching the existing idiom — no inotify). On a new request: write status `joining` → tear down any current association → feed the new credentials into the existing association machinery for one bounded attempt → write `joined` or `failed` (+ the nl80211 reason, as precisely as it reports — wrong-passphrase often looks like a handshake timeout; pass the reason through verbatim, don't over-claim).
- After the terminal status, the new credentials stay current and the existing backoff/watch loop owns them (epic decision 6 — no revert). An unparseable request file → status `failed` with a parse error, then ignore it until the id/bytes change (self-healing, gosd-6cf2 lesson).
- **Decision 5 restructure:** the watcher must run even when boot found no credentials. Today `wifiup.Run` returns immediately with none configured — restructure so on a WiFi-capable board the watcher always runs and the association loop idles until credentials exist. A board with no WiFi interface fails a request honestly ("no WiFi interface").
- Keep the seam: pure logic + fakes on macOS in wifiup.go; real nl80211 stays in platform_linux.go (teardown/associate already exist — reuse, don't add netlink code). Remember the `netlink.Request`-flag trap if any netlink call IS touched (wifiup/connect_linux_test.go pins the pattern).
- Redaction: when a request is read, register its passphrase for crash-report redaction alongside boot/sequence.go's existing STA rule (epic decision 7).
- mDNS: on a successful join, Notify the same Changed signal wifiup already threads (should need zero mdnsresponder changes — verify, per gosd-qfbk decision 7).

## Persist (epic decision 3)

On `joined` with `Persist: true`, write `config/wifi/ssid` and `config/wifi/passphrase` into the card's config tree via the same write path the cloud-init seed consumption uses (find it in the seed-consumption code before writing anything new — padding discipline and durable-write ordering live there). Verify the boot partition's mount state allows the write the way the seed path does. Never persist on failure.

## Wiring (cmd/gosd-init/main.go)

- Thread a restart signal from wifiup's join-success into the cloudflared and tsfunnel `Deps` added by the sibling bean. Every successful runtime join fires it, even same-SSID (epic decision 4). No baked ingress → signal goes nowhere, fine.
- StartNetworking passes the reconciler what it needs (config-tree write access for persist, the signal, the mdns Signal already threaded).

## Tests

Fake-driven, macOS-passing, behavioral: request → joining/joined status sequence; failure reason surfaced; persist writes the tree only on success and only when asked; no-boot-credentials board still serves a request; ingress signal fired on success (incl. same-SSID) and not on failure; unparseable request self-heals. An end-to-end test pairing the real `wifi.Join` public API against the reconciler through a temp dir would be the strongest single test — prefer it over many small mocks.

## Notes

- Branch `bean/gosd-uy4x-wifiup-runtime-join` from main. "Part of gosd-ojbm" in the PR body. Needs a `.changeset/*.md` change file (minor — the feature's user-facing behavior lands here; mention wifi.Join, per-call persist, and the automatic ingress reconnect).
- Run every quality gate in CLAUDE.md (both golangci-lint invocations) before pushing; foreground `gh pr checks <n> --watch --interval 30` after.
- Do not merge the PR — JP reviews and merges.
