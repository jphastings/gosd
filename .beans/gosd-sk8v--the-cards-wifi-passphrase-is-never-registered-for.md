---
# gosd-sk8v
title: The card's WiFi passphrase is never registered for crash-report redaction, unlike the ingress tokens
status: todo
type: bug
priority: low
created_at: 2026-08-20T05:04:29Z
updated_at: 2026-08-20T05:04:29Z
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

- [ ] Register the card’s and the baked WiFi passphrase as redaction rules
- [ ] Test: a report whose detail contains the configured passphrase does not carry it
- [ ] Check nothing else operator-supplied is still outside the rule set
