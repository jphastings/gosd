---
# gosd-4fve
title: Shim binary cmd/gosd-tsfunnel + the tailscale.com dependency
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:08:25Z
updated_at: 2026-08-07T15:08:28Z
parent: gosd-65uy
blocked_by:
    - gosd-virc
---

Tailscale epic gosd-65uy bean 2 — the isolated big-dependency PR and the
epic's highest-risk item; FIRST in build order once the epic unblocks. No
cloudflared sibling dependency (blocked only by the strict-after gate).

## Locked decisions

- New `cmd/gosd-tsfunnel` in the MAIN module (nested-module rejection: epic
  decision 1). ~150 lines. Flags: --statedir, --hostname,
  --backend http://localhost:<port>, --funnel-port (443), --register-timeout
  (5m). TS_AUTHKEY via env ONLY — argv is logged by the supervisor, env
  never (mergeUserEnv precedent).
- tsnet.Server{Dir: statedir, Hostname}; ListenFunnel("tcp", ":<funnel-port>")
  → httputil.ReverseProxy to the backend. Proxy contract: KEEP the inbound
  Host header (apps see the public *.ts.net name), X-Forwarded-For default,
  explicitly set X-Forwarded-Proto=https.
- tsnet Logf DISCARDED (would flood a 115200 serial console); UserLogf →
  stderr (the supervisor prefixes it).
- Fatal errors exit nonzero with actionable wrapping: funnel-unavailable
  errors from ListenFunnel name all three tailnet-side prereqs (funnel
  nodeAttr in the policy file, HTTPS certs enabled, MagicDNS) + point at
  docs/ingress.md; --register-timeout expiry (tsnet not Running in time)
  names key expiry and a wrong clock as the suspects. The supervisor's
  backoff owns all retry — capmap changes propagate, so retry genuinely
  heals a tailnet policy fixed after boot.
- go.mod/go.sum delta measured and RECORDED IN THIS BEAN before merge: go
  directive bump 1.26.4→1.26.5, dep count, MVS bumps (x/net, x/sys,
  x/crypto), first module-download size. Full quality gates + the qemu boot
  CI job run on this PR alone before anything builds on it.
- Shim binary size measured with and without the ts_omit tag set (epic
  decision 2), recorded here.

## Todos

[ ] go get tailscale.com (pinned); record the dependency delta in this bean
[ ] shim main: flags, tsnet setup, ListenFunnel, ReverseProxy contract, register timeout
[ ] unit tests for the pure parts (flag parsing, proxy director, error mapping) — pass on macOS
[ ] size measurement with/without ts_omit tags, recorded in this bean
