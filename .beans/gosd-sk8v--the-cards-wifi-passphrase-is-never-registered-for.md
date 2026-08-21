---
# gosd-sk8v
title: The card's WiFi passphrase is never registered for crash-report redaction, unlike the ingress tokens
status: completed
type: bug
priority: low
created_at: 2026-08-20T05:04:29Z
updated_at: 2026-08-21T07:55:11Z
---

**Severity: Low.** The same latent gap bean gosd-tzd1 closed for the ingress
credentials, one setting over — found while doing that work, deliberately
left out of its PR because it is a different secret with a different label.

`wifi/passphrase` on the card is an operator-supplied secret that gosd-init
reads and holds in memory (`cardWifi` in cmd/gosd-init/main.go, then
`wifiup.ConfigCredentials`), and it is not among the redaction rules a crash
report is scrubbed with. After gosd-tzd1 the rule set is:

- `envRedactionRules(userEnv)` — every app env value
- `ingressRedactionRules(config)` — the Cloudflare token and Tailscale
  authkey
- `fault.RegisterSecretString` registrations

The WiFi passphrase, and `config.json`’s baked `Wifi.Passphrase` fallback,
are in none of them.

## Why it might matter

Same reasoning as gosd-tzd1: nothing in `wifiup` prints it today (its
logging discipline is good — SSIDs are logged, the passphrase is not), so
this is a safety net rather than a live leak. But the redaction system
exists so nobody has to audit each new log line, and a WPA2 passphrase is
the credential most likely to be shared with other devices on the same
network — and most likely to be reused elsewhere by the person who chose
it.

## Fix sketch

A rule alongside `ingressRedactionRules`, over both the card’s
`wifi/passphrase` and `initcfg.Config.Wifi.Passphrase`, replacing the value
with `{wifi: passphrase}`. Note the length floor: a passphrase can legally
be 8 characters, which is exactly `redact.MinNeedleLength`, so a short one
is right at the boundary and a shorter one (never valid for WPA2-PSK) is
skipped and logged by label.

## Todos

- [x] Register the card’s and the baked WiFi passphrase as redaction rules
- [x] Test: a report whose detail contains the configured passphrase does not carry it
- [x] Check nothing else operator-supplied is still outside the rule set


## Summary of Changes

`wifiRedactionRules` (cmd/gosd-init/internal/boot/sequence.go) registers the
WiFi passphrase from both places one can come from — the card's
`wifi/passphrase` setting and config.json's baked `Wifi.Passphrase`, the same
two sources `wifiup.ConfigCredentials` resolves — each replaced with
`{wifi: passphrase}`, the label this bean's fix sketch specified. Wired into
the same seam as gosd-tzd1's ingress rules, at the same moment in `Run`:

```go
secrets := envRedactionRules(userEnv)
secrets = append(secrets, ingressRedactionRules(config)...)
secrets = append(secrets, wifiRedactionRules(config, cfg)...)
report.setSecrets(secrets)
```

Both sources are registered whichever one the device ends up joining with:
what decides whether a value can reach a report is that gosd-init read it
into memory this boot, not which of them won the precedence contest.

**The SSID is deliberately not a rule.** It is broadcast to anyone in radio
range, gosd-init logs it on purpose, and redacting it would remove the one
detail that makes a WiFi failure diagnosable. Recorded here so it reads as a
decision rather than an omission.

**The length floor**, which the bean flagged: WPA2-PSK's own minimum is 8
characters and `redact.MinNeedleLength` is 8, so every legal passphrase is
redacted — the boundary case is included, not excluded (`New` skips
`len(needle) < 8`). A shorter one is only reachable by hand-editing the card;
`redact.New` skips it and reports it by label, so the console says a value was
left alone without saying which. Nothing is silently dropped. A pre-hashed
64-hex PSK in the same field is far above the floor and covered as-is.

### Tests

- `TestWifiRedactionRulesCoverBothPlacesAPassphraseComesFrom`
- `TestWifiRedactionRulesIgnoreTheSSIDAndAnUnsetPassphrase` — an open network
  (SSID set, no passphrase) must produce no rule, or the console logs a
  skipped `{wifi: passphrase}` for a device that has no passphrase.
- `TestRunRedactsBothWifiPassphrasesFromAnAppCrashReport` — drives the whole
  of `Run` with the app printing both passphrases to its own stdout, and
  asserts the written `LAST_FATAL_ERROR.md` carries the placeholder and
  neither value. Verified as a real gate: it fails against the unwired code.
- `TestRunRedactsAWizardSuppliedWifiPassphraseFromAnAppCrashReport` — the
  Imager wizard is the flagship way an operator supplies a passphrase, and it
  is covered only because `consumeCloudInit` writes the seed into the tree
  (sequence.go:337) *before* `setSecrets` reads it (sequence.go:437). Moving
  either would silently stop scrubbing the passphrase most devices are given,
  with every other test still green, so the ordering is asserted rather than
  assumed.

Documented in the crash-report guide's Secrets section.

## The third todo: the sweep

Every value gosd-init reads into memory, and where each one now stands.

**Covered by a rule.**

- `env/<NAME>` (card) and `Config.Env` (baked) — `envRedactionRules`, over
  `mergeUserEnv`'s output. gosd-init's reserved `GOSD_BOARD`/`GOSD_HOSTNAME`/
  `GOSD_DATA_FLUSH` are excluded on purpose (gosd-m6py).
- `ingress/cloudflared/token`, `ingress/tailscale-funnel/authkey` —
  gosd-tzd1.
- `wifi/passphrase` (card) and `Config.Wifi.Passphrase` (baked) — this bean.
- Anything an app hands to `fault.RegisterSecretString`, re-read fresh at
  every report.

**No rule, and each for a reason worth stating.**

- `wifi/ssid`, `hostname` — operator-supplied but not secret. Both are
  announced to the local network (the SSID by radio, the hostname by mDNS)
  and both are logged deliberately; scrubbing either would blank the device's
  own identity from a report about that device.
- `ingress/*/hostname` — the public URL the tunnel serves. Publishing it is
  the whole feature.
- `ingress/*/port`, `funnel_port`, `data_flush` — small integers and a
  boolean. Below `MinNeedleLength`, and redacting them is exactly the
  confetti failure that floor exists to prevent (`PORT=80` blanking every
  "80" in a byte count).
- `Config.Board`, `Identity`, `BoardDisplayName`, `DataLabel`,
  `DataFilesystem`, `NTPServers`, `ConfigDigests`, `BuildTimestamp`, the two
  ingress-enabled booleans, and the `gosd.*` kernel cmdline args — build
  metadata, not credential material. `Board` is in the report header by
  design.
- `Config.AppName`, `AppVersion`, `SupportURL` — not needles; they are
  already scrubbed as *haystack* (faultreport's `scrub`), which is the
  correct direction for them.
- Cloud-init networks past the first. `cloudInitValues` writes only
  `result.Wifi[0]` into the tree, so a second network's password is never
  registered — but the `provision.Result` holding it is created inline at
  sequence.go:337, passed by value, and unreachable the moment
  `consumeCloudInit` returns. Nothing retains it, so no later `%+v` can print
  what no longer exists. Covered by scope rather than by rule, and the seed
  file itself is durably deleted in the same call.

**One real gap, and a recorded decision not to close it with a rule.**

An app may ship settings *outside* `env/` — the settings guide explicitly
invites it ("nothing stops you shipping a whole new top-level file or
directory for a device-level setting your app reads directly off the card"),
and its own example name is `google-service-account.json`. Such a value is
read into `cardconfig.Tree`, persisted by `configstore`, and has no redaction
rule. It is not reachable through gosd-init today: gosd-init acts only on
paths it knows, and every `log()` in `cardconfig` and `configstore` names a
path (`OnCard(rel)`), never a value — but the app reads it off the read-only
`/boot` itself, and if the app prints it, it lands in the console tail that
becomes a report's technical detail.

Registering every unrecognised tree value was considered and rejected: gosd
cannot tell a credential from a theme name, so a blanket rule would blank
every occurrence of an ordinary short value across the report — and would log
skipped labels naming innocuous settings as secrets. The floor that protects
`env/` works because those values are known to be the app's; it has no such
warrant here.

So the answer is the app-facing one, and the fix was to make it *findable*
rather than to add a rule: both the settings guide and the crash-report guide
now say plainly that a setting outside `env/` is not swept, and point at
`fault.RegisterSecretString`. The asymmetry was real and undocumented — a
developer could reasonably have assumed the whole config tree was covered
because half of it is. Worth JP's eye on whether a naming convention (a
`secrets/` subtree, swept wholesale) is wanted later; that would be a design
decision, not a bug fix, and this bean does not make it.
