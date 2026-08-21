---
# gosd-65uy
title: 'Tailscale Funnel ingress: gosd build --ingress tailscale-funnel (all boards)'
status: completed
type: epic
priority: normal
created_at: 2026-08-07T15:07:24Z
updated_at: 2026-08-21T01:41:31Z
blocked_by:
    - gosd-wxjy
---

Second ingress agent, after the cloudflared epic (gosd-virc). A tsnet-based
shim gives gosd devices a public https://<hostname>.<tailnet>.ts.net URL via
Tailscale Funnel, reverse-proxying to a local app port. Full plan:
~/.claude/plans/we-re-currently-implementing-providing-crispy-meteor.md
(research + design session 2026-08-07).

STRICTLY AFTER cloudflared (JP decision, 2026-08-07): this epic is blocked by
gosd-virc end-to-end — nothing here starts until the cloudflared epic ships
through its bench bean gosd-igk0. The five cloudflared bean amendments that
keep the --ingress rail generic for two agents (Render(Ingress) whole-table;
registry-shaped flag parse; shared logwriter/childbackoff packages;
table-driven provsnapshot classification; docs/ingress.md overview structure)
never reached the implementing agents (they lived only in a working-tree
stash while the backlog burndown ran); recovered verbatim from stash@{0} on
2026-08-07 into the refactor bean [[gosd-wxjy]], which now blocks this epic
alongside the bench gate. AMENDED by JP 2026-08-08: software beans may
proceed ahead of the cloudflared bench pass (gosd-igk0); only this epic's
own bench bean gosd-79v8 remains hardware-gated.

## Locked decisions

1. **tsnet shim compiled FROM GOSD SOURCE** (cmd/gosd-tsfunnel, MAIN module),
   NOT upstream tailscaled blobs: Funnel cannot be configured via env/file
   (TS_SERVE_CONFIG is containerboot-only), so a shell-less init would need
   the 30MB tailscale CLI talking to the daemon socket; and tailscaled has a
   history of dying at startup without an iptables binary
   (tailscale/tailscale#17623). tsnet is pure Go userspace netstack: no
   /dev/net/tun, no root, no iptables; ListenFunnel configures Funnel
   in-process. NESTED-MODULE ALTERNATIVE REJECTED on a hard ground: Go module
   zips exclude nested-go.mod dirs, so source-ladder rung 2 (go mod download
   github.com/jphastings/gosd@version, internal/build/gosdinit.go) would not
   contain the shim source. Do not relitigate.
2. **Build tag set**: every ts_omit_* from tailscale.com/feature/featuretags
   EXCEPT netstack/serve/acme/bakedroots (required), plus -ldflags="-s -w" —
   shim only; gosd-init's build argv stays byte-identical. ts_omit_ssh
   compiles Tailscale SSH out entirely: that is this epic's "no interactive
   surface" compliance argument (cf. gosd-virc decision 7 for cloudflared).
3. **State on /data is mandatory**: tsnet Dir = /data/.gosd/tailscale/ (the
   documented gosd-init bookkeeping namespace, docs/runtime.md). Losing
   tailscaled.state = new node identity = new public URL. Actionable build
   error when --ingress tailscale-funnel has no data partition;
   --data-size=expand is fine (dataexpand runs before StartNetworking).
   Read-only /data at runtime → one actionable line + return. Designed
   upside: identity on GOSD-DATA means a plain Imager reflash keeps the same
   node and URL with zero re-auth — strictly better than cloudflared's story.
4. **Runbook mandates a TAGGED, REUSABLE auth key**: tagged nodes get
   node-key expiry auto-disabled (the 180-day tailnet default would brick
   shipped devices). Auth keys expire ≤90 days but are needed only for FIRST
   registration — tsnet ignores the key once state exists (operators may
   delete it from gosd.toml afterwards).
5. **Plain ListenFunnel** (tailnet peers + internet), no FunnelOnly, no knob
   in v1: the URL is public anyway; with userspace netstack the only
   reachable endpoint is this listener; tailnet reachability is the
   operator's debug path while fixing a misconfigured funnel nodeAttr.
   FunnelOnly can become an opt-in key later without breaking anything.
6. **gosd.toml**: [ingress.tailscale-funnel] with authkey (secret — never
   logged), port (required, local app port), optional hostname (defaults to
   the device hostname), optional funnel_port (443 default; allowed set
   {443, 8443, 10000}).
7. **Go-directive policy**: tailscale.com bumps go.mod go 1.26.4 → 1.26.5
   (release-notes line for library consumers; module graph pruning keeps
   their impact small). Pin tailscale.com; never accept a pin bump whose go
   floor exceeds the released Go toolchain.

## Funnel facts (verified 2026-08-07)

Ports 443/8443/10000 only, TCP. TLS terminates ON-NODE (Let's Encrypt via
Tailscale ACME; ingress relays route on TLS SNI and cannot read traffic).
Tailnet-side prereqs the device cannot set for itself: MagicDNS, HTTPS certs
enabled, funnel nodeAttr in the policy file. Available on all plans incl.
free. Unquantified bandwidth caps; no custom domains. GOARM=6 self-compile
covers pi-zero-w (upstream even ships GOARM=5; 32-bit Go crypto slow but
functional, tailscale/tailscale#7053) → ALL boards supported, contrast
cloudflared's arm64-only.

## Summary of Changes

`gosd build --ingress tailscale-funnel` ships on every board, giving a device
a public `https://<hostname>.<tailnet>.ts.net` URL with no listener on any
real host interface. gosd-4fve added `cmd/gosd-tsfunnel` — a tsnet shim in
the MAIN module (the nested-module route was rejected because Go module zips
exclude nested-`go.mod` directories, which would have emptied the source
ladder's `go mod download` rung) — and the `tailscale.com` dependency;
gosd-kzd3 built the build rail: the flag in the agent registry,
`build.CrossCompileTsfunnel` compiling the shim once per architecture the
selected boards need, bundling to `/bin/gosd-tsfunnel`, and the
data-partition gate that refuses the flag without `--data-size` because
losing `tailscaled.state` means a new node identity and a new public URL.
gosd-e3mm wrote the runtime module and gosd-o68e wired it live into
`StartNetworking` under the PID-1 reaper, exactly like cloudflared.
gosd-85bn and gosd-u2gz covered the config schema and its provisioning
classification — since carried over wholesale to the config tree
(`config/ingress/tailscale-funnel/`) by epic gosd-rw6n. gosd-1cqa wrote the
docs and the COMPATIBILITY row, and gosd-79v8 proved it end to end against a
real tailnet on a NanoPi Zero2.

The "no interactive surface" argument holds by construction: `ts_omit_ssh`
compiles Tailscale SSH out entirely, and tsnet's userspace netstack dials
out over WireGuard rather than binding a socket.

Two hard-won facts outlived the epic and are recorded in CLAUDE.md: a
gosd-shipped subprocess gets a minimal env with no `HOME` and no
`os.UserCacheDir()`, so a library's directory env must be set explicitly
(gosd-6cf2); and a build-tag feature trim silently broke tsnet's
control-plane registration on-device but not on macOS (gosd-h46e), which is
why the shim ships full tsnet.

Closing note: `docs/ingress.md` still carried a "not yet hardware-verified"
caveat contradicting COMPATIBILITY.md's bench footnote; corrected in the
same PR as this status change.
