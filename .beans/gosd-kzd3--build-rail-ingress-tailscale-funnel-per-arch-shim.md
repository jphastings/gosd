---
# gosd-kzd3
title: 'Build rail: --ingress tailscale-funnel, per-arch shim compile, data-size gate'
status: todo
type: task
priority: normal
created_at: 2026-08-07T15:08:45Z
updated_at: 2026-08-07T15:08:51Z
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

[ ] crossCompileInDir opts + CrossCompileTsfunnel (assert gosd-init argv unchanged)
[ ] compileForBoards third binary + ExtraExecutables + initcfg field
[ ] flag/validate case + data-size hard error + collision entry, unit tests
[ ] gosd run --data-size
[ ] integration test with/without the flag
