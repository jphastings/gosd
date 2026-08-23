---
# gosd-e721
title: Send the pending upstream patches (go-diskfs FAT sizing + label trim, pion/mdns leak)
status: in-progress
type: task
priority: normal
created_at: 2026-08-03T18:30:55Z
updated_at: 2026-08-23T16:13:53Z
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

## Additional upstream candidate (2026-08-08, from bean gosd-6cf2; mechanism corrected 2026-08-23)

tailscale.com/tsnet wedges when a state file is present but unparseable.
IMPORTANT — the original "no write→rename" framing here was WRONG (see
gosd-6cf2's 2026-08-23 correction): tsnet writes both tailscaled.state and
tailscaled.log.conf ATOMICALLY (atomicfile.WriteFile, write→fsync→rename,
since 2022), so there is no torn-write window. The defect is a SELF-HEAL gap:
store.NewFileStore regenerates an EMPTY tailscaled.state but treats a
NON-EMPTY unparseable one as fatal, and tsnet's startLogger treats an empty
OR corrupt tailscaled.log.conf as fatal. A corrupt file therefore only arises
from underlying media corruption (FAT32 /data, no journal, SD FTL corrupting
completed sectors on power loss). On appliances whose state dir survives
reflash (our /data) that wedge is unrecoverable without out-of-band deletion.
Upstream fix worth proposing: self-heal an unparseable state file (treat as
absent → fresh registration) — NOT write→rename, which is already done. We
work around it in cmd/gosd-tsfunnel for now (drop-if-not-valid-JSON,
gosd-6cf2). Prepare the patch/rationale in a local clone; do NOT open a PR
upstream without JP.


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

- **go-diskfs sectors-per-FAT under-computation** — one-line numerator fix
  that closes TWO INDEPENDENT defects on the same line, not one: (A) a missing
  `+8*sectorsPerCluster` numerator term that yields a self-inconsistent FAT
  (first defective whole-GiB size is 3 GiB, in the 4 KB cluster class); and
  (B) the uint16 truncation plus uint32 numerator overflow that mis-sizes
  above ~256 GiB and panics at exactly 512 GiB. Describe them as two: the
  earlier "uint16 cast corrupts past 3 GiB with the >32 GB cluster class"
  phrasing FUSED them and is wrong — the 3 GiB figure belongs to defect A, not
  to the uint16 cast. Full derivation, proof sweep and the two sender caveats
  are in [[gosd-e3e3]]'s Summary of Changes; carry those caveats into the PR
  description rather than presenting the fix as unconditional.
- **go-diskfs volume-label per-field 8.3 trim** (interior byte-7 space loss) —
  accurate as prepared; sketched patch (trim the concatenated 11 raw bytes
  once, for the volume-label entry) in [[gosd-f83b]]'s Summary. Frame it as a
  READ-side bug: go-diskfs mis-reads its own spec-correct write, so any other
  OS reads the label fine. It CANNOT affect GoSD's own image labels
  (`<prefix>-boot`/`-data`/`-conf` can't hold a space at byte 7), but it CAN
  affect the disk/emmc public API's app-supplied FAT32 labels — which is
  exactly what GoSD's workaround (internal/blockmount's ValidateLabel byte-7
  rule) guards.
- **pion/mdns v2.1.0 socket leak** — accurate as prepared; leaks its two
  internal unicast sockets (2 fds per failed `NewServer`) on `Server()`'s
  early error returns. Exact `conn.go` line references and the suggested patch
  are in [[gosd-o6tp]]'s Summary. Check the finding still applies at pion/mdns
  HEAD before writing it up; the references are to v2.1.0. It is a REAL leak
  but effectively harmless in GoSD's usage: a normal boot leaks 2 fds once,
  recovered on the first DHCP lease; only a sustained interface-flap could
  accumulate, and the 250ms restart floor already bounds the rate. Worth
  sending, not urgent; no GoSD-side change is required. (An optional
  belt-and-braces would be gating `restart()` on a usable interface actually
  existing — record as an option only, do NOT file a bean for it.)
- **tailscale.com/tsnet state-file wedge** — see the section above; the fix is
  SELF-HEAL PARITY, not write→rename (atomic writes are already done upstream
  since 2022). NewFileStore already treats an EMPTY tailscaled.state as missing
  (upstream issue #895); the patch extends that to treat a non-empty
  unparseable one the same way. CRITICAL for JP before sending: the prepared
  patch fixes ONLY tailscaled.state (ipn/store). The tailscaled.log.conf wedge
  is SEPARATE — it lives in tsnet.startLogger, not ipn/store, and is arguably
  worse (fatal even on an EMPTY file, which the state path already handles) —
  so the patch as prepared does NOT close the whole issue. Say so in the PR.
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
