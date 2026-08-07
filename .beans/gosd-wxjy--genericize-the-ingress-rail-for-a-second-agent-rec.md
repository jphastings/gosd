---
# gosd-wxjy
title: Genericize the --ingress rail for a second agent (recovered pre-merge amendments)
status: completed
type: task
priority: normal
created_at: 2026-08-07T20:20:19Z
updated_at: 2026-08-07T21:38:43Z
---

Recovered content (2026-08-07): JP amended five cloudflared beans during the
Tailscale Funnel design session so the --ingress rail would come out generic
for a second agent — but the amendments lived only in the main checkout's
working tree (later a stash), and the backlog-burndown agents implemented the
un-amended versions from main. The merged code works but is cloudflared-shaped.
This bean carries those amendments as a post-merge refactor. Source of truth:
`stash@{0}` on the main checkout (base ec738e3) and
~/.claude/plans/we-re-currently-implementing-providing-crispy-meteor.md.
Pure refactor — NO behavior change; every existing test keeps passing with
mechanical updates only.

## Todos (one per recovered amendment)

[x] gosdtoml: `Render()` takes the WHOLE `Ingress` table, not
    `IngressCloudflared` — future agents add sections without another
    signature change at every call site (pipeline + provsnapshot re-render).
[x] cmd/gosd: `--ingress` parses into a registry-shaped selection of known
    agent names (unknown-value error LISTS valid agents); `validateIngress`
    validates PER-AGENT — each agent contributes its own board rule and its
    own reserved dests to the `--with-external` collision list (cloudflared:
    /bin/cloudflared + the CA path; board rule unchanged).
[x] gosd-init: extract the line-splitting prefix writer and restart backoff
    into tiny SHARED packages — cmd/gosd-init/internal/logwriter and
    cmd/gosd-init/internal/childbackoff, prefix/policy as constructor args —
    identical content and tests to today's in-package files. The supervise
    loop itself stays per-agent (deliberate small duplication; revisit only
    if a third agent appears).
[x] provsnapshot: classification becomes TABLE-DRIVEN over the gosdtoml
    Ingress struct — each [ingress.<agent>] section classifies whole-section
    via one table row, so a new agent adds a row + tests, not new logic.
[x] docs/ingress.md: open with a short "choosing an ingress" overview — per
    agent: what it is, board support, where TLS terminates, whose account you
    need — with per-agent sections underneath (the cloudflared runbook
    becomes the first such section).

Blocks the Tailscale Funnel epic's implementation beans (gosd-85bn,
gosd-kzd3, gosd-e3mm, gosd-u2gz, gosd-1cqa) from being clean single-purpose
diffs; the epic gosd-65uy references this bean.



## Summary of Changes

- `internal/gosdtoml/template.go`: `Render()`'s last parameter is now the
  whole `Ingress` table (not just `IngressCloudflared`); it reads
  `ingress.Cloudflared` internally. Both call sites
  (`internal/pipeline/pipeline.go`, `cmd/gosd-init/internal/provsnapshot`)
  updated to pass `gosdtoml.Ingress{}`/the whole table. Golden-output tests
  in `internal/gosdtoml/template_test.go` assert the exact same rendered
  bytes as before (only the Go call-site literals changed, from
  `IngressCloudflared{...}` to `Ingress{Cloudflared: ...}`) — this is the
  no-behavior-change proof for this todo.
- `cmd/gosd/ingress.go`: `--ingress` now parses into a small
  `ingressAgents` registry (name, per-board capability rule, reserved
  `--with-external` dests) via `parseIngressFlags`/`findIngressAgent`;
  unknown values list every valid agent name. `validateIngress` loops over
  the registry and validates each *selected* agent independently
  (`validateIngressAgent`), producing byte-identical
  `--ingress cloudflared failed: ...` wording since `agent.name` is still
  "cloudflared". `cmd/gosd/external.go`'s `reservedExternalDests` is now
  built by merging `baseReservedExternalDests` (/init, /app) with every
  registered agent's `reservedDests` map, so /bin/cloudflared and the CA
  bundle path stay reserved unconditionally, unchanged in practice.
  `cmd/gosd/build.go`/`run.go` updated for `ingressSelection`'s
  `.Cloudflared` field (renamed local var `ingressCloudflared` ->
  `ingressSelected` for clarity).
- `cmd/gosd-init/internal/logwriter` (new) and
  `cmd/gosd-init/internal/childbackoff` (new): the line-splitting prefixed
  writer and the exponential-backoff-with-cap engine, moved out of
  `cmd/gosd-init/internal/cloudflared` verbatim, with prefix (logwriter) and
  base/max (childbackoff, already a constructor arg) as the generic
  parameters. `cloudflared.Deps.NewBackoff`/`runOnce` now use
  `*childbackoff.Backoff`; `runOnce` builds its stdout/stderr writers via
  `logwriter.New("cloudflared: ", deps.Log)`. cloudflared's own
  `backoff.go`/`backoff_test.go` keep only its policy constants
  (`DefaultBackoffBase`/`DefaultBackoffCap`/`StableAfter`) and the test that
  pins them. `cmd/gosd-init/main.go` updated to wire
  `childbackoff.NewBackoff`. The supervise loop itself stays in
  `cloudflared.go`, per the locked decision.
- `cmd/gosd-init/internal/provsnapshot/provsnapshot.go`: ingress
  classification (fresh/snapshot intent, restore, apply, effective, decode)
  is now table-driven over an `ingressSections` table (one row today:
  Cloudflare Tunnel), each row a `configured`/`restore` function pair
  operating on the whole `gosdtoml.Ingress` table. `Provisioning.Ingress`
  changed type from `gosdtoml.IngressCloudflared` to `gosdtoml.Ingress`
  (mechanical test literal updates in `provsnapshot_test.go` at the two
  spots that constructed a `Provisioning` or compared `.Ingress` directly).
  Restore log wording is unchanged ("restoring Cloudflare Tunnel ingress
  settings from the provisioning snapshot") since the one row's label is
  "Cloudflare Tunnel".
- `docs/ingress.md`: added a "Choosing an ingress" overview (what an
  ingress is, a per-agent board-support/TLS-termination/account table, and
  the shared shape every agent follows), followed by the existing content
  renamed to a "Cloudflare Tunnel (`gosd build --ingress cloudflared`)"
  section with all its subsections demoted one heading level (##->###,
  ###->####); no wording inside that section changed.

No new features or config keys. Proof of no behavior change: every existing
test passes with only mechanical updates (Go type/literal changes, no
assertion changes) — see the per-file notes above; full gate output
(go test ./..., go vet, gofmt, golangci-lint x2) is clean.
