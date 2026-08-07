---
# gosd-wxjy
title: Genericize the --ingress rail for a second agent (recovered pre-merge amendments)
status: todo
type: task
created_at: 2026-08-07T20:20:19Z
updated_at: 2026-08-07T20:20:19Z
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

[ ] gosdtoml: `Render()` takes the WHOLE `Ingress` table, not
    `IngressCloudflared` — future agents add sections without another
    signature change at every call site (pipeline + provsnapshot re-render).
[ ] cmd/gosd: `--ingress` parses into a registry-shaped selection of known
    agent names (unknown-value error LISTS valid agents); `validateIngress`
    validates PER-AGENT — each agent contributes its own board rule and its
    own reserved dests to the `--with-external` collision list (cloudflared:
    /bin/cloudflared + the CA path; board rule unchanged).
[ ] gosd-init: extract the line-splitting prefix writer and restart backoff
    into tiny SHARED packages — cmd/gosd-init/internal/logwriter and
    cmd/gosd-init/internal/childbackoff, prefix/policy as constructor args —
    identical content and tests to today's in-package files. The supervise
    loop itself stays per-agent (deliberate small duplication; revisit only
    if a third agent appears).
[ ] provsnapshot: classification becomes TABLE-DRIVEN over the gosdtoml
    Ingress struct — each [ingress.<agent>] section classifies whole-section
    via one table row, so a new agent adds a row + tests, not new logic.
[ ] docs/ingress.md: open with a short "choosing an ingress" overview — per
    agent: what it is, board support, where TLS terminates, whose account you
    need — with per-agent sections underneath (the cloudflared runbook
    becomes the first such section).

Blocks the Tailscale Funnel epic's implementation beans (gosd-85bn,
gosd-kzd3, gosd-e3mm, gosd-u2gz, gosd-1cqa) from being clean single-purpose
diffs; the epic gosd-65uy references this bean.
