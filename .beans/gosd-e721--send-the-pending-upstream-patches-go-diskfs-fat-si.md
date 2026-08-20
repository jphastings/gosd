---
# gosd-e721
title: Send the pending upstream patches (go-diskfs FAT sizing + label trim, pion/mdns leak)
status: todo
type: task
priority: normal
created_at: 2026-08-03T18:30:55Z
updated_at: 2026-08-20T04:58:29Z
---

Three upstream fixes were written up during the review sweep but not
sent, per the no-third-party-PRs rule — JP decides and sends:

- go-diskfs sectors-per-FAT under-computation: one-line numerator fix
  with full derivation, proof sweep, and two caveats for the sender —
  recorded in bean gosd-e3e3's Summary of Changes.
- go-diskfs volume-label per-field 8.3 trim (interior byte-7 space
  loss): sketched patch (trim the concatenated 11 raw bytes once for
  the volume-label entry) in bean gosd-f83b's Summary.
- pion/mdns v2.1.0 leaks its two internal unicast sockets on Server()'s
  early error returns: exact conn.go line references and suggested
  patch in bean gosd-o6tp's Summary.

GoSD carries local mitigations for all three, so none is urgent; sending
them retires the mitigations' upstream halves eventually.

---

## Additional upstream candidate (2026-08-08, from bean gosd-6cf2)

tailscale.com/tsnet writes its JSON state files (tailscaled.state, the node
identity; tailscaled.log.conf, the persistent log ID) without a write→rename,
so a power cut mid-write leaves an empty/truncated file that tsnet.Up refuses
to load ("unexpected end of JSON input") and never regenerates — a permanent
wedge. On appliances whose state dir survives reflash (our /data) this is
unrecoverable without out-of-band deletion. Upstream fix worth proposing:
tsnet should either write these atomically (temp + rename) or self-heal an
unparseable state file (treat as absent → fresh registration). We work around
it in cmd/gosd-tsfunnel for now (drop-if-not-valid-JSON, gosd-6cf2). Prepare
the patch/rationale in a local clone; do NOT open a PR upstream without JP.


---

## DECISION (JP, 2026-08-20): prepare all four, ready for JP to send

JP sends them; an agent prepares them. All four get a real branch in a local
clone with the patch applied and the project's own test suite run, plus a
ready-to-paste PR title and description. JP reviews and presses send.

**The no-third-party-PRs rule still binds absolutely.** Preparing a patch in
`~/src/ext/<project>` is permitted; opening a PR, pushing a branch to the
upstream repo, or forking it under any account is NOT — that applies to
subagents too, and a fork-and-push counts as opening a PR. If a project's
contribution guide demands a fork before a patch can even be tested, stop at
the local branch and say so in this bean.

None is urgent: GoSD carries a local mitigation for every one. Sending them
retires the mitigations' upstream halves eventually, and that is the whole
value — these are not blocking anything.

### Per-patch preparation notes

- **go-diskfs sectors-per-FAT under-computation** — one-line numerator fix.
  Full derivation, proof sweep and the two sender caveats are already in
  [[gosd-e3e3]]'s Summary of Changes; carry those caveats into the PR
  description rather than presenting the fix as unconditional.
- **go-diskfs volume-label per-field 8.3 trim** (interior byte-7 space loss) —
  sketched patch (trim the concatenated 11 raw bytes once, for the volume-label
  entry) in [[gosd-f83b]]'s Summary.
- **pion/mdns v2.1.0 socket leak** — leaks its two internal unicast sockets on
  `Server()`'s early error returns. Exact `conn.go` line references and the
  suggested patch are in [[gosd-o6tp]]'s Summary. Check the finding still
  applies at pion/mdns HEAD before writing it up; the references are to v2.1.0.
- **tailscale.com/tsnet state-file wedge** — see the section above. Two
  candidate fixes: atomic write→rename, or self-heal an unparseable state file
  by treating it as absent. Propose the self-heal as primary: it also repairs
  devices already wedged in the field, which write→rename alone cannot.
  Tailscale is the most process-heavy of the four upstreams — read their
  contribution guide and CLA position before writing anything, and note in the
  PR description that this was found on an appliance whose state dir survives
  reflash, which is what makes it unrecoverable rather than merely annoying.

### Todo

- [ ] Prepare go-diskfs FAT-sizing patch in `~/src/ext/go-diskfs` (branch + tests + PR text)
- [ ] Prepare go-diskfs label-trim patch (same clone, separate branch)
- [ ] Re-verify the pion/mdns leak at HEAD, then prepare the patch
- [ ] Prepare the tsnet state-file patch; check Tailscale's CLA/contribution process
- [ ] Report back with the four branches and PR texts for JP to send
- [ ] Once any is merged upstream, file a follow-up to retire GoSD's local mitigation
