---
# gosd-85bn
title: 'gosdtoml: [ingress.tailscale-funnel] schema + Render example block'
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:07:49Z
updated_at: 2026-08-08T04:05:33Z
parent: gosd-65uy
blocked_by:
    - gosd-7upw
    - gosd-virc
---

Tailscale epic gosd-65uy bean 1 (after cloudflared's gosd-7upw lands the
Ingress table and Render(Ingress) signature).

## Locked decisions

- Sibling struct in internal/gosdtoml:
  `IngressTailscaleFunnel { Authkey, Hostname string; Port, FunnelPort int }`
  + `Configured() bool` (any field non-zero); field on Ingress is
  `TailscaleFunnel` with toml tag "tailscale-funnel" (TOML bare keys allow
  dashes — no quoting needed in user files).
- Coercion mirrors gosd-7upw exactly: the two strings coerce-with-warning
  from bare scalars; the two ints accept int64 or quoted all-digits;
  warnings name ONLY the key, NEVER the value (authkey is a secret —
  mergeUserEnv precedent); deterministic sorted warning order.
- Semantic validation (port range, funnel_port set membership, hostname
  defaulting) lives in the runtime module, NOT Parse (validHostname
  precedent; matches the cloudflared split).
- Render example block appended after cloudflared's, plain-language per the
  file's voice: authkey from the admin console (tagged + reusable — the tag
  disables node-key expiry), port = the local HTTP service, hostname
  defaults to the device hostname, and the comment states BOTH the
  `gosd build --ingress tailscale-funnel` requirement AND that the authkey
  is only needed for first registration (safe to remove afterwards).

## Todos

[x] Struct/rawConfig/coercion + warnings, tests (incl. an authkey-never-in-warnings scan)
[x] Render example block + golden test



## Summary of Changes

- `internal/gosdtoml/config.go`: added `IngressTailscaleFunnel{Authkey,
  Hostname string; Port, FunnelPort int}` + `Configured()`, wired as
  `Ingress.TailscaleFunnel` (toml tag `tailscale-funnel`). Generalized
  `coerceIngressString`/`coerceIngressPort` to take a table name
  parameter (was cloudflared-only) so both providers share one coercion
  path with identical behavior/messages for cloudflared (byte-for-byte
  unchanged) and the new
  `gosd.toml [ingress.tailscale-funnel] <key> ...` messages for
  tailscale-funnel. `coerceIngress` now delegates to
  `coerceIngressCloudflared` and the new `coerceIngressTailscaleFunnel`,
  concatenating warnings (cloudflared's, then tailscale-funnel's) for a
  deterministic, non-sorted order matching struct field order — mirrors
  gosd-7upw exactly. Semantic validation (port range, funnel_port set
  membership, hostname defaulting) is explicitly left to the future
  tsfunnel runtime module, not Parse (validHostname/cloudflared precedent).
- `internal/gosdtoml/template.go`: added
  `ingressTailscaleFunnelCommentedOut`/`ingressTailscaleFunnelTemplate`,
  rendered by `Render()` right after the cloudflared block. The commented
  example explains authkey (from the tailnet admin console, tagged +
  reusable, only needed for first registration and safe to remove after),
  port (the local app port) and hostname (defaults to the device
  hostname), and states the `gosd build --ingress tailscale-funnel`
  requirement up front. funnel_port is rendered too (both forms) even
  though the bean's prose doesn't dwell on it: `Render()` is the same
  function `provsnapshot`'s self-heal re-render calls with the whole
  `gosdtoml.Ingress` struct (verified by reading
  `cmd/gosd-init/internal/provsnapshot`, out of this bean's scope to
  modify), so omitting a struct field from the template would silently
  drop that field's value on the very next re-render.
- `internal/gosdtoml/config_test.go`: parse table-test cases for
  `[ingress.tailscale-funnel]` (full config, missing table, both ingress
  tables at once, bare-scalar coercion, fixed warning order regardless of
  file order, non-scalar drops, bad quoted port, partial-parse survival),
  plus `TestTailscaleFunnelWarningsNeverIncludeTheAuthkeyValue`
  (mirrors the existing token-scan test) and
  `TestIngressTailscaleFunnelConfigured`.
- `internal/gosdtoml/template_test.go`: golden tests for both rendered
  forms (`TestRenderIngressTailscaleFunnelExactOutputWith[out]Values`),
  a round-trip test with all four fields set, and
  `TestRenderBothIngressBlocksTogether` guarding the appended-after-
  cloudflared ordering. Updated the two pre-existing cloudflared golden
  tests from `strings.HasSuffix` to `strings.Contains`, since
  cloudflared's block is no longer the last thing Render emits. Also
  changed the tailscale-funnel example's placeholder hostname from
  "my-device" to "device-name" — it collided with the top-level
  hostname example value and broke
  `TestRenderWithValuesRoundTripsThroughParse`'s commented-hostname
  check.

Gates run: `go test ./internal/gosdtoml/...`, `go test ./...`,
`go vet ./...`, `gofmt -l .`,
`golangci-lint run --allow-parallel-runners ./...` (both host and
`GOOS=linux`) — all clean.
