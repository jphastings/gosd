---
# gosd-tzd1
title: Ingress tokens are never registered for crash-report redaction, unlike every app env value
status: todo
type: bug
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-12T04:18:42Z
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

- [ ] `ingressRedactionRules` over Token and Authkey, wired into `setSecrets`
- [ ] Test: a report whose detail contains the configured tunnel token does not carry it
- [ ] Decide and record whether the two supervised children's relayed output should be redacted on the console
