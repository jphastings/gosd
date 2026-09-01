---
# gosd-uy4x
title: 'wifiup: runtime join reconciler, per-call persist, main.go wiring'
status: completed
type: task
priority: normal
created_at: 2026-09-01T15:28:18Z
updated_at: 2026-09-01T17:14:27Z
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

## Summary of Changes

- **`cmd/gosd-init/internal/wifiup`**: restructured `Run` (decision 5) so the
  runtime-join watcher (new `runtimejoin.go`) always starts on a
  WiFi-capable board, even with no boot credentials and even with a nil
  `WifiClient` (decision 8 — a board with no WiFi hardware still answers a
  request, honestly, with "no WiFi interface"). A `credState` holds the
  creds currently in effect (boot's, or the latest runtime request's) and a
  channel that interrupts the association loop when they change, so a new
  request tears down the old association and starts fresh (decision 6 — no
  revert after a terminal status). `runAssociationLoop` grew an optional
  `firstOutcome` callback fired exactly once, the moment the FIRST
  association attempt of a call settles — this needed threading through
  `runUntilDisconnect`/`watchAssociation` too, because that pair only
  returns on loss or shutdown by design (it exists to maintain a healthy
  connection, not merely confirm one), so a naive "call it after the
  function returns" hook would never fire for a successful join. The
  watcher self-heals an unparseable request.json (reports failure once,
  ignores it until the bytes change) and dedupes on request id otherwise.
  `credentials.go` gained `resolveCredentials`, factored out of
  `ConfigCredentials.Credentials` so a runtime request's ssid/passphrase
  resolves through the exact same hex-PSK/DerivePSK logic boot credentials
  do.
- **Persist** (decision 3): `Deps.Persist func(ssid, passphrase string) error`,
  called only from the outcome callback on a successful join whose request
  asked for it. Wired in `main.go` (`wifiPersist`) through
  `platform.EditBootPartition` + the boot-time-read `cardconfig.Tree`'s own
  `Write` — the identical remount/write/remount-back path
  `boot.consumeCloudInit` uses for a cloud-init seed, so the padding
  discipline pads against the tree's already-known on-card content rather
  than a fresh zero value.
- **Redaction** (decision 7): `Deps.RegisterSecret`, called the moment the
  watcher reads a request with a non-empty passphrase. Wired to
  `registerRuntimeWifiSecret` in `main.go`, which does a read-merge-write
  against `/run/gosd/secrets.json` (`internal/secretreg`'s format) rather
  than a blind overwrite — gosd-init isn't the only writer of that file
  (the app's own `wifi.Join` already writes to it via
  `fault.RegisterSecretString`), so clobbering it would discard whatever
  the app itself had already registered. Belt and suspenders: it protects
  a request that reached `/run/gosd/wifi` without ever going through
  `wifi.Join`.
- **Ingress restart** (decision 4): `Deps.RestartIngress`, called after
  every successful join, same-SSID included. `main.go` now builds two
  `*restartsignal.Signal`s (one each for cloudflared/tsfunnel — sharing one
  would let whichever consumer reads it first silently swallow the
  notification the other never sees) and fires both from
  `wifiRestartIngress`; each ingress package's `Deps.Kill` is wired to its
  own `Kill` function. A signal nobody's listening to (the ingress that
  isn't baked) is a no-op, per `restartsignal.Signal`'s own contract.
- **mDNS**: verified, not changed. A runtime join reuses
  `runAssociationLoop`/`runUntilDisconnect`/`onLeaseFor` unmodified for the
  DHCP/lease-apply step, and `wifiupDeps`'s `MarkNetworkUp` closure already
  calls `mdnsChanged.Notify()` — so a successful runtime join already
  restarts the mDNS responder through the exact mechanism a boot-time
  association does, with zero mdnsresponder changes.
- **`main.go`**: `wifiup.NewPlatform()` failing no longer skips wifiup
  entirely — `Run` is always started (with a possibly-nil `WifiClient`) so
  the watcher can serve an honest failure. `wifiup.Options.RequestDir` is
  wired to `wifictl.Dir`.
- Three pre-existing tests (`TestRunSkipsEverythingWhenNoCredentialsConfigured`,
  `TestRunLogsAndSkipsOnCredentialError`, `TestRunSkipsUnsupportedSecurity`)
  had to move from a synchronous `Run(deps, Options{})` call to a
  goroutine + `Stop` channel, since `Run` no longer returns immediately in
  those cases (decision 5's whole point).
- New tests in `runtimejoin_test.go`, including the bean's suggested
  strongest single test: a real `wifiup.Run` reconciler driven end to end
  through a real temp directory via `internal/wifictl` directly (writing a
  request, polling status) — the closest a test in this package can get to
  exercising the real `wifi.Join` public API, which is gated off entirely
  under the `!gosd` build tag `go test` always uses, and which an internal
  package can't reach into anyway (Go's own internal-import-path rule).
  Also covers: joining→joined sequence, verbatim failure reason, persist
  gated on success+asked, a no-boot-credentials board still serving a
  request, a nil-`WifiClient` board failing honestly, same-SSID still
  firing the ingress restart (and a failed join never firing it), and the
  unparseable-request self-heal.
- `.changeset/wifi-runtime-join-reconciler.md` (minor).

Not done (deliberately out of scope for this bean): decision 9's "Join
fails fast against an active AP" clause — gosd-qfbk's WiFi AP mode epic has
not landed in code yet (only bean files exist), so this epic lands first
and that clause is gosd-qfbk's responsibility when it lands second.
COMPATIBILITY.md and docs are gosd-kg73's job per the epic's own child-bean
order, not this one's.
