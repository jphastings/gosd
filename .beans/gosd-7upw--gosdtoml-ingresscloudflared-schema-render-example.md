---
# gosd-7upw
title: 'gosdtoml: [ingress.cloudflared] schema + Render example block'
status: todo
type: task
created_at: 2026-08-07T12:51:33Z
updated_at: 2026-08-07T12:51:33Z
parent: gosd-virc
---

First bean of the ingress epic (gosd-virc — read its locked decisions). Schema
only; no consumer yet.

## Locked decisions

- `internal/gosdtoml/config.go`:
  `Ingress struct { Cloudflared IngressCloudflared `+"`toml:\"cloudflared\"`"+` }`
  (a table, not inline — future agents get sibling tables without a schema
  break); `IngressCloudflared { Token, Hostname string; Port int }` + a
  `Configured() bool` (any field non-zero).
- Lenient rawConfig coercion per existing style: the three strings
  coerce-with-warning from bare scalars; `port` accepts int64 or quoted
  all-digits (data_flush mirror-leniency); anything else dropped with warning.
  Warnings name ONLY the key, NEVER the value — the token is a secret
  (mergeUserEnv precedent). Deterministic sorted warning order.
- Semantic validation (FQDN shape, port range, missing keys) does NOT live in
  Parse — it belongs to the runtime module (validHostname precedent).
- `Render()` gains an `ingress IngressCloudflared` param (provsnapshot re-render
  needs real values; zero value renders the commented example). Call sites:
  internal/pipeline/pipeline.go ~L209 (zero value) + provsnapshot render paths.
- Example block: appended after [env], present in EVERY image (the comment
  itself states the gosd build --ingress requirement), plain-language per the
  file's existing voice: token from `cloudflared tunnel token <name>` or the
  dashboard, hostname = public domain, port = local HTTP service.

## Todos

[ ] Config/rawConfig/coercion + warnings, tests (incl. a token-never-in-warnings scan)
[ ] Render param + example block golden test; call sites updated
