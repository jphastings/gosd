---
# gosd-kzd3
title: 'Build rail: --ingress tailscale-funnel, per-arch shim compile, data-size gate'
status: completed
type: task
priority: normal
created_at: 2026-08-07T15:08:45Z
updated_at: 2026-08-08T05:59:34Z
parent: gosd-65uy
blocked_by:
    - gosd-85bn
    - gosd-4fve
    - gosd-g4km
---

Tailscale epic gosd-65uy bean 3 (after TS-1 gosd-85bn, TS-2 gosd-4fve, and
cloudflared's gosd-g4km, which lands the registry-shaped --ingress rail).

## Locked decisions

- Generalize crossCompileInDir (internal/build/gosdinit.go) with a small
  opts struct {tags, ldflags}; CrossCompileGosdInit passes empty opts — its
  build argv must stay BYTE-IDENTICAL (image identity is content-derived;
  gosd-init is never stripped). New build.CrossCompileTsfunnel pins the
  ts_omit tag set + -ldflags="-s -w" (epic decision 2). Same 3-rung source
  ladder incl. --gosd-init-src; document that the flag now covers both
  from-source binaries.
- Third per-arch compile in compileForBoards (cmd/gosd/archbuild.go) beside
  the initPaths map, ONLY when the selection includes tailscale-funnel. ALL
  boards, incl. pi-zero-w (GOARM=6 — we control the compile, unlike
  cloudflared's GOARM=7 upstream asset). Dest /bin/gosd-tsfunnel via
  ExtraExecutables; initcfg.Config gains IngressTailscaleFunnel bool
  (json "ingressTailscaleFunnel,omitempty").
- validateIngress tailscale-funnel case: every board passes the board rule;
  HARD ERROR when no data partition (--data-size absent/0):
  "tailscale-funnel stores its tailnet identity on the GOSD-DATA partition;
  pass --data-size (e.g. --data-size=64MiB or --data-size=expand)".
  --data-size=expand is fine. /bin/gosd-tsfunnel joins the --with-external
  collision list (per-agent reserved dests, gosd-g4km's registry).
- gosd run (cmd/gosd/run.go) gains --data-size passthrough — it currently
  hardcodes size 0 (run.go:131), so qemu could never have a writable /data;
  needed for the TS-5 runtime smoke.
- Integration test: with the flag → /bin/gosd-tsfunnel 0755 in the image +
  config.json flag + gosd.toml example block; without → none of the three.
  NOTE: this test compiles real tailscale code for arm64 (module/build cache
  makes repeat runs tolerable); the network tripwire covers gosd's OWN
  fetches, not `go build` subprocesses — no fixture seam needed, unlike
  cloudflared's fake-ELF fixture.

## Todos

[x] crossCompileInDir opts + CrossCompileTsfunnel (assert gosd-init argv unchanged)
[x] compileForBoards third binary + ExtraExecutables + initcfg field
[x] flag/validate case + data-size hard error + collision entry, unit tests
[x] gosd run --data-size
[x] integration test with/without the flag



## Summary of Changes

- `internal/build/gosdinit.go`: generalized `crossCompileInDir` with a
  `crossCompileOpts{tags, ldflags}` struct and a pure `buildGoBuildArgs`
  helper; `CrossCompileGosdInit` passes the zero value at all three rungs,
  keeping its `go build` argv byte-identical to before — pinned by
  `TestBuildGoBuildArgsOmitsTagsAndLdflagsWhenOptsIsZero`.
- `internal/build/tsfunnel.go` (new): `CrossCompileTsfunnel`, the same
  3-rung ladder (dev checkout, module cache, `--gosd-init-src` override) as
  `CrossCompileGosdInit`, pinned to the epic's 74-tag `ts_omit_*` set
  (generated from `tailscale.com/feature/featuretags.Features`, excluding
  netstack/serve/acme/bakedroots) plus `-ldflags="-s -w"`. The override rung
  derives gosd-tsfunnel's directory as a `gosd-tsfunnel` sibling of the given
  `--gosd-init-src` (which is documented as pointing at gosd-init's own leaf
  package dir, e.g. the nix flake's `$out/share/gosd-src/cmd/gosd-init`) — a
  design call made because a single directory can't simultaneously be two
  different main packages; flagging this for review since the bean didn't
  spell out the mechanism. `internal/build/tsfunnel_test.go` (new): real
  arm64 + GOARM=6 compiles, a strip check (no .symtab/.debug_info), the
  override/sibling-derivation path, and a fast tag-set assertion.
- `cmd/gosd/ingress.go`: added `tailscaleFunnelAgent` to `ingressAgents`
  (name `tailscale-funnel`, matching gosdtoml's table name) — `capableGOARCH`
  always true (gosd compiles it itself, unlike cloudflared's upstream
  GOARM=7-only asset) and `reservedDests` claims `/bin/gosd-tsfunnel`.
  `ingressAgent` gained `requiresDataPartition string`; new
  `validateIngressDataPartition` refuses a build/run selecting an agent with
  this set when `--data-size` is absent/0 and `--data-size=expand` wasn't
  passed, with the bean's exact wording. `ingressSelection` gained
  `TailscaleFunnel bool`. New `openTsfunnelBinary` opens the
  compileForBoards-produced binary (no ELF pre-flight needed — it's this same
  invocation's own toolchain output, not a downloaded/cached blob).
- `cmd/gosd/archbuild.go`: `compileForBoards` gained `needsTsfunnel bool`
  and a `compileTsfunnel` func param, adding a third per-arch compile
  (deduped like gosd-init's) only when `--ingress tailscale-funnel` was
  selected; `archBinaries` gained `tsfunnelPath`.
- `cmd/gosd/build.go` / `cmd/gosd/run.go`: wire
  `ingressSelected.TailscaleFunnel` into `compileForBoards`'s new params,
  open the compiled binary into `ExtraExecutables` at
  `ingressTailscaleFunnelDest`, set `pipeline.Options.IngressTailscaleFunnel`,
  and call `validateIngressDataPartition` alongside the existing
  `validateIngress` call. `gosd run` gained `--data-size` (previously
  hardcoded to 0), threading `DataExpand` through too — needed so `gosd run
  --ingress tailscale-funnel` (and the later TS-5 runtime smoke) can satisfy
  the data-partition requirement. Deliberately NOT routed through
  `sharedcontent.go` (see that file's updated doc comment): the shim is
  per-arch COMPILED by `compileForBoards`, like the app and gosd-init, not
  fetched/cached like the CA bundle and cloudflared, so build/run parity
  comes from both commands sharing `compileForBoards` rather than from
  `sharedContent`.
- `internal/initcfg/config.go`: `Config` gained
  `IngressTailscaleFunnel bool `json:"ingressTailscaleFunnel,omitempty"``,
  mirroring `IngressCloudflared`.
- `internal/pipeline/pipeline.go`: `Options` gained
  `IngressTailscaleFunnel bool`, threaded into the `initcfg.Config` literal
  `Assemble` builds.
- Tests: `cmd/gosd/ingress_test.go` (new, unit), `ingress_integration_test.go`
  (embed+config+data-partition-refusal+expand-accepted), `run_integration_test.go`
  (same for `gosd run`), `buildrun_parity_integration_test.go` (extended to
  pass both `--ingress cloudflared --ingress tailscale-funnel --data-size` and
  assert `/bin/gosd-tsfunnel` on both sides — proves two independent
  `go build` subprocess compiles of the shim, moments apart, are
  byte-reproducible), `archbuild_test.go` (third-binary dedup/skip/failure
  cases), `external_test.go` and the reserved-dest collision test extended
  to the new dest, `internal/pipeline/pipeline_test.go` and
  `internal/initcfg/config_test.go` unit tests. Per the bean's note, the
  build/run integration tests compile real `tailscale.com` code for arm64 and
  GOARM=6 (no fixture seam needed — `noNetworkTransport`/`disableNetwork`
  cover gosd's own fetches, not `go build` subprocesses).
- `COMPATIBILITY.md`: added the Tailscale Funnel ingress row (✅ on every
  board) and a footnote pointing at the epic's bench bean `gosd-79v8`.
- `docs/ingress.md` deliberately left untouched: its per-agent runbook
  needs the runtime supervisor wiring (a sibling bean, not yet landed) to be
  usable end-to-end; the overview table/help text already reflect the new
  flag value via `ingressAgentNames()`.
