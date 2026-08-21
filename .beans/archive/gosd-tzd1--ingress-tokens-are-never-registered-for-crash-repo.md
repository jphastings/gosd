---
# gosd-tzd1
title: Ingress tokens are never registered for crash-report redaction, unlike every app env value
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-20T05:02:15Z
---

**Severity: Medium.** A missing safety net rather than an active leak — but
it is the one class of secret gosd-init holds *itself*, and it is the only
one the redaction system does not know about.

## Verified

The crash-report redaction pipeline is fed from exactly two sources:

- `envRedactionRules(userEnv)` at
  `cmd/gosd-init/internal/boot/sequence.go:448` — scoped to `userEnv`, i.e.
  gosd.toml's `[env]` and config.json's `Env`. **Never `Ingress`.**
- `fault.RegisterSecretString`, an app-facing API that `cmd/gosd-init` never
  calls for itself.

So `gosdtoml.IngressCloudflared.Token` and
`gosdtoml.IngressTailscaleFunnel.Authkey` — values gosd-init reads from the
card and holds in memory — are never registered as redaction rules.

Every app env value is scrubbed from `LAST_FATAL_ERROR.md` automatically.
The tunnel token and Tailscale authkey are not.

## Why it matters

A Cloudflare tunnel token lets an attacker impersonate the device's public
endpoint. A Tailscale auth key lets them join nodes to the user's tailnet.
The crash report is a file the device writes to a FAT partition and whose own
text instructs the reader to *"send them this whole file"* — users forward it
to support, paste it into GitHub issues, and photograph it.

Today no gosd-init code path embeds these values in a log line or error — I
confirmed that separately across `cloudflared/`, `tsfunnel/`,
`gosd-tsfunnel/` and `gosdtoml`'s coercion layer, and the discipline is
genuinely well kept (`TS_AUTHKEY` travels in env, never argv; `decodeToken`'s
error paths report only byte offsets and JSON type info). So this is not
currently exploitable.

It is a latent gap: the moment any future change adds a `%+v` of a config
struct, or an upstream library logs its own credential, the value reaches the
card unredacted — whereas an app env value in the identical position would be
scrubbed. The redaction system exists precisely so that nobody has to audit
each new log line.

## Related — the serial console has no redaction at all

`redact.Redact` has exactly one call site in the repo:
`internal/faultreport/faultreport.go:168`. Every other log line, including
cloudflared's and gosd-tsfunnel's relayed stdout/stderr forwarded verbatim
through `internal/logwriter`, is unfiltered. That is deliberate for the
console as a physically-attached debug channel (a locked decision in bean
gosd-m6py), and this bean does not propose changing it — but it does mean
these two third-party children's own output is the one place a token could
surface with no defense whatsoever. Worth a decision, recorded either way.

## Fix

Feed the resolved ingress secrets into the same rule set, at the point
`gosdToml.Ingress` is resolved in `sequence.go`:

```go
report.setSecrets(append(envRedactionRules(userEnv), ingressRedactionRules(gosdToml.Ingress)...))
```

with replacements naming the field, not the value (`{ingress: cloudflared-token}`),
matching `redact.Rule`'s existing contract. Note the replacement text must be
sanitised — see the crash-report Markdown-injection bean.

## Todos

- [x] `ingressRedactionRules` over Token and Authkey, wired into `setSecrets`
- [x] Test: a report whose detail contains the configured tunnel token does not carry it
- [x] Decide and record whether the two supervised children's relayed output should be redacted on the console

## Summary of Changes

Fixed together with gosd-ywsv and gosd-fu1z in one PR — see gosd-ywsv for the
single rule the three of them settle on.

- `ingressRedactionRules` (cmd/gosd-init/internal/boot/sequence.go) turns
  the card's `ingress/cloudflared/token` and
  `ingress/tailscale-funnel/authkey` into rules replacing each value with
  `{ingress: cloudflared-token}` / `{ingress: tailscale-funnel-authkey}` —
  the field's name, never its value. Wired into the existing seam:
  `report.setSecrets(append(envRedactionRules(userEnv),
  ingressRedactionRules(config)...))`.
- Built where `Run` has the settled config tree, not inside the ingress
  agents: an agent that never starts (no binary baked, network never up,
  child dead on arrival) must not be the reason its token stays in a
  report. A setting nobody set contributes no rule at all, so an
  unconfigured card doesn't log a skip naming a credential it doesn't have.
- Note on the bean's suggested source: the tokens no longer come from
  `gosd.toml` — epic gosd-rw6n replaced it with the config tree — so the
  rules read `cardconfig.Tree` paths. Same values, same moment in the boot
  sequence.
- Tests: `TestIngressRedactionRulesNameEachCredentialByItsField`,
  `TestIngressRedactionRulesIgnoresCredentialsNobodySet`, and
  `TestRunRedactsTheCardsTunnelTokenFromAnAppCrashReport` — the last drives
  the whole of `Run`, with the app printing the configured token to its own
  stdout, and asserts the written `LAST_FATAL_ERROR.md` carries the
  placeholder and not the token.
- Documented in the crash-report guide's Secrets section, alongside the env
  sweep.

## Decision: the supervised children's relayed output stays unredacted

Recorded as this bean asked, with no code change.

`cloudflared` and `gosd-tsfunnel` have their stdout/stderr wrapped in
`logwriter.New(prefix, deps.Log)`, and `deps.Log` is gosd-init's console
logger. That output reaches the serial console and **nowhere else**: the
console tail that becomes a report's technical detail is teed from `/app`'s
own stdout/stderr alone (sequence.go's `appOutput`), never from what
gosd-init logs itself. So there is no path by which a child's line reaches
the card today, and the console is deliberately unredacted per gosd-m6py's
locked decision — a physically-attached debug channel for someone already
holding the board, not a file that travels.

Redacting the console would also cost the thing it exists for: it is the
one place a developer can see what a child actually printed, and it is the
channel gosd-init itself debugs through.

Should that ever change — a future path that folds a supervised child's
output into a report — this bean's rules now cover it by construction,
which is the point: the token is registered at the moment it is read, not
at the moment something decides to print it. The decision is recorded in
the crash-report guide's "serial console is never redacted" bullet, which
now names the two children explicitly.
