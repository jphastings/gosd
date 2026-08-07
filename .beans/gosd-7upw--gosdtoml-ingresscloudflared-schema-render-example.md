---
# gosd-7upw
title: 'gosdtoml: [ingress.cloudflared] schema + Render example block'
status: completed
type: task
priority: normal
created_at: 2026-08-07T12:51:33Z
updated_at: 2026-08-07T16:47:35Z
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

[x] Config/rawConfig/coercion + warnings, tests (incl. a token-never-in-warnings scan)
[x] Render param + example block golden test; call sites updated



## Summary of Changes

- `internal/gosdtoml/config.go`: added `Ingress`/`IngressCloudflared` schema
  (`Config.Ingress`, a table-of-tables so future providers get sibling tables),
  `Configured() bool`, and `rawIngress`/`rawIngressCloudflared` for lenient
  decoding. Added `coerceIngress`/`coerceIngressString`/`coerceIngressPort`/
  `isAllDigits`: token/hostname coerce bare scalars to text with a warning,
  port accepts a bare int64 or quoted all-digit string with a warning,
  anything else is dropped with a warning — every warning names only the key,
  never the value (token is a secret; the whole table shares that discipline
  rather than special-casing just token). Warning order is fixed
  (token, hostname, port), independent of file order. Semantic validation
  (FQDN shape, port range, required-together keys) is deliberately NOT here —
  the doc comment points at the future cloudflared runtime module.
- `internal/gosdtoml/template.go`: `Render()` gained an
  `ingress IngressCloudflared` parameter, appended after [env]. Zero value
  renders a commented, plain-language example (states the
  `gosd build --ingress cloudflared` requirement up front, since — unlike
  WiFi/[env] — a hand-edit here does nothing without that build flag);
  `Configured()` renders the real token/hostname/port.
- Call sites updated: `internal/pipeline/pipeline.go` passes a zero
  `IngressCloudflared{}` (no `--ingress` build flag exists yet — later stack
  member's job). `cmd/gosd-init/internal/provsnapshot/provsnapshot.go`: the
  `heal()` restore-write now threads `cfg.Ingress` through `plan.apply` so an
  unrelated hostname/WiFi/[env] restore can't blank out a hand-edited
  [ingress.cloudflared] table on the card; the snapshot's own `encode()` passes
  the zero value with a comment, since `Provisioning`/`Snapshot` don't carry
  Ingress yet (that round-trip is the later provsnapshot child bean in the
  epic, gosd-virc).
- Tests: `internal/gosdtoml/config_test.go` — table-driven `Parse` cases for
  full/partial ingress config, bare-scalar coercion, non-scalar drops, the
  quoted-non-digit port case, warning-order determinism regardless of file
  order, and a malformed-entry-doesn't-block-hostname case; plus a dedicated
  `TestIngressWarningsNeverIncludeTheTokenValue` that plants a distinctive
  marker in several malformed token shapes and scans every warning for it,
  and a `Configured()` table test.
  `internal/gosdtoml/template_test.go` — golden tests for both rendered forms
  (`TestRenderIngressExactOutputWithoutValues`,
  `TestRenderIngressExactOutputWithValues`) and a round-trip test
  (`TestRenderWithIngressRoundTripsThroughParse`); existing Render call sites
  updated for the new parameter, and one pre-existing assertion
  (`TestRenderWithValuesRoundTripsThroughParse`) tightened from a bare
  "# hostname" substring check to the specific commented line, since the new
  ingress example legitimately contains its own "# hostname = ..." text.

## Surprises / notes for reviewers

- The full `go test ./...` gate could not be closed out 100% clean in this
  environment: `cmd/gosd`'s heavy board-matrix integration suite
  (`build_integration_test.go`, ~39 tests doing real cross-compiles and image
  writes) hit `no space left on device` from a shared-machine disk that was
  at 93-99% capacity throughout this session (many sibling agents building
  concurrently). This reproduced identically across three separate runs, on
  completely unrelated stdlib packages (crypto/tls, net/url, mime) and even
  bare `t.TempDir()` calls with no relation to gosdtoml — clear evidence of
  environment exhaustion, not a code defect (matches the documented
  contention pattern in this repo's CLAUDE.md). Mitigating evidence instead:
  full `go test ./...` passed clean for every OTHER package including
  `internal/gosdtoml`, `internal/pipeline`, and
  `cmd/gosd-init/internal/provsnapshot`; and the three `cmd/gosd` tests that
  most directly exercise gosd.toml rendering
  (`TestBuildBakesEnvFlagsIntoConfigJSONAndGosdToml`,
  `TestBuildIdentityChangesWithHostnameAndWifiViaGosdToml`,
  `TestBuildRejectsReservedEnvKeyActionably`) were run in isolation and
  passed. `go vet ./...`, `gofmt -l .`, and both `golangci-lint run ./...`
  invocations (darwin and `GOOS=linux`) are clean.
