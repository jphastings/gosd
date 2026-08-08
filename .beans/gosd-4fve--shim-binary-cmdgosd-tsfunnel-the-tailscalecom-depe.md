---
# gosd-4fve
title: Shim binary cmd/gosd-tsfunnel + the tailscale.com dependency
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:08:25Z
updated_at: 2026-08-08T04:53:02Z
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

[x] go get tailscale.com (pinned); record the dependency delta in this bean
[x] shim main: flags, tsnet setup, ListenFunnel, ReverseProxy contract, register timeout
[x] unit tests for the pure parts (flag parsing, proxy director, error mapping) — pass on macOS
[x] size measurement with/without ts_omit tags, recorded in this bean



## Measurements (recorded before merge, per locked decision)

**go directive:** `go 1.26.4` → `go 1.26.5` — forced by tailscale.com
v1.102.2's own `go.mod` floor (`go 1.26.5`). go1.26.5 is a released stable
toolchain (confirmed against go.dev/dl before pinning), so the epic's "never
accept a pin bump whose go floor exceeds the released Go toolchain" gate is
satisfied; `GOTOOLCHAIN=auto` downloaded it transparently (no manual step
needed, ~231MB one-time toolchain cache cost on the build machine, not a
project dependency).

**Dependency count delta** (`go.mod` require blocks, direct vs. indirect):

| | before | after | delta |
|---|---|---|---|
| direct requires | 20 | 21 | +1 (`tailscale.com`) |
| indirect requires | 22 | 56 | +34 |
| total `go.mod` requires | 42 | 77 | +35 |
| `go.sum` lines | 113 | 313 | +200 |
| full module graph (`go list -m all`) | 143 | 619 | +476 |

**Notable MVS bumps** (existing deps pulled forward by tailscale.com's own
requirements):

- `golang.org/x/net` v0.55.0 → v0.56.0
- `golang.org/x/sys` v0.46.0 → v0.47.0
- `golang.org/x/crypto` v0.53.0 → v0.54.0
- `github.com/klauspost/compress` v1.19.0 → v1.19.1
- `github.com/spf13/cobra` v1.10.1 → v1.10.2 (+ `pflag` v1.0.9 → v1.0.10)
- `golang.org/x/mod` newly appears in the graph at v0.37.0

**First `go mod download` size** (module cache cost of the 35 newly-required
modules — everything tsnet's import graph actually needs, not tailscale.com's
full CLI/GUI dependency set):

- Compressed download (`.zip` files): ~20.3 MiB
- Extracted source on disk: ~56.0 MiB
- For reference, a fully-cold `go mod download` of this module's entire
  77-entry require list (i.e. every dependency, not just the new ones) totals
  ~183.4 MiB of `.zip` downloads.

**Shim binary size** (`GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build
-ldflags="-s -w"`, measured then deleted immediately — not committed):

| Build | Size |
|---|---|
| WITH the epic's `ts_omit_*` tag set (74 tags: every omittable `tailscale.com/feature/featuretags` feature except `netstack`/`serve`/`acme`/`bakedroots`) | 15,990,946 bytes (~15.25 MiB) |
| WITHOUT any `ts_omit_*` tags | 20,709,538 bytes (~19.75 MiB) |
| Savings | 4,718,592 bytes (~4.5 MiB, ~23%) |

The 74-tag set (including `ts_omit_ssh`, the epic's "no interactive surface"
compliance argument) was generated programmatically from
`tailscale.com/feature/featuretags.Features`, filtering to omittable tags
outside `{netstack, serve, acme, bakedroots}`, rather than hand-transcribed —
avoids drift if upstream adds/renames a feature tag in a later pin bump.

**gosd-init leak check:** `go list -deps ./cmd/gosd-init | grep -c tailscale`
→ `0`. Confirmed the new dependency is isolated to `cmd/gosd-tsfunnel` and
does not reach gosd-init's build.

## Summary of Changes

- `go.mod`/`go.sum`: added `tailscale.com` v1.102.2 (direct), which bumped
  the go directive to 1.26.5 and pulled in 34 new indirect modules (tsnet's
  transitive closure: gVisor netstack, wireguard-go, etc.) plus a handful of
  MVS bumps to already-present deps — see Measurements above.
- `cmd/gosd-tsfunnel/main.go` (new): the binary's entry point — parses flags,
  builds a `tsnet.Server` (state dir, hostname, `Logf` discarded, `UserLogf`
  to stderr), calls `Server.Up` under a `--register-timeout` context before
  `ListenFunnel` so a stuck registration is reported as *that* failure rather
  than as a Funnel error, then serves a reverse proxy over the Funnel
  listener. `run` takes `(args, stderr)` so `main` stays a thin, untested
  wrapper.
- `cmd/gosd-tsfunnel/flags.go` (new): `parseFlags` — `--statedir`,
  `--hostname`, `--backend`, `--funnel-port` (default 443, restricted to
  443/8443/10000), `--register-timeout` (default 5m). Deliberately no
  `--authkey` flag: `TS_AUTHKEY` reaches `tsnet.Server` via its own
  environment-variable fallback (`Server.AuthKey` left empty), since
  gosd-init's supervisor logs child argv but never child environment.
- `cmd/gosd-tsfunnel/proxy.go` (new): `newReverseProxy` builds the
  `httputil.ReverseProxy` — `Rewrite` sets the backend URL, preserves the
  inbound Funnel hostname on `Out.Host`, applies `SetXForwarded`'s defaults,
  then overrides `X-Forwarded-Proto` to `https` explicitly (the connection
  this process accepts from tsnet's Funnel listener is already-decrypted
  plain HTTP, so TLS-based inference would get this backwards).
- `cmd/gosd-tsfunnel/errors.go` (new): `registerTimeoutError` and
  `funnelUnavailableError` wrap the two tsnet failure modes with the
  actionable text the epic locks — expired `TS_AUTHKEY`/wrong clock for the
  former, the three tailnet ACL/HTTPS/MagicDNS prerequisites plus
  `docs/ingress.md` for the latter.
- `cmd/gosd-tsfunnel/{flags,proxy,errors}_test.go` (new): behavioral unit
  tests — flag parsing (valid/override/error cases, `--help`, and that no
  `--authkey`-shaped flag exists), the proxy's header contract via
  `httptest`, and that both error wrappers `errors.Is` their cause and
  mention the documented remediation text. No real tailnet involved.
- `.beans/gosd-65uy--*.md`: removed `gosd-virc` from `blocked_by` (kept
  `gosd-wxjy`) and appended JP's 2026-08-08 amendment to the "STRICTLY AFTER
  cloudflared" paragraph — software beans may now proceed ahead of the
  cloudflared bench pass; only this epic's own bench bean (`gosd-79v8`)
  stays hardware-gated.
