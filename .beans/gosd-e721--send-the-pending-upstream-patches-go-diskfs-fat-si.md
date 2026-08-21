---
# gosd-e721
title: Send the pending upstream patches (go-diskfs FAT sizing + label trim, pion/mdns leak)
status: in-progress
type: task
priority: normal
created_at: 2026-08-03T18:30:55Z
updated_at: 2026-08-21T00:00:00Z
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

- [x] Prepare go-diskfs FAT-sizing patch in `~/src/ext/go-diskfs` (branch + tests + PR text)
- [x] Prepare go-diskfs label-trim patch (same clone, separate branch)
- [x] Re-verify the pion/mdns leak at HEAD, then prepare the patch
- [x] Prepare the tsnet state-file patch; check Tailscale's CLA/contribution process
- [x] Report back with the four branches and PR texts for JP to send
- [ ] Once any is merged upstream, file a follow-up to retire GoSD's local mitigation


---

## Summary of Changes (2026-08-21) — all four prepared, NOT sent

All four patches exist as local branches with the change applied, the
project's own tests run, and ready-to-paste PR wording written. **Nothing was
pushed, forked or opened anywhere.** Sending them is JP's, and only JP's.

Each clone carries a `PROPOSED-PR.md` at its root with the suggested title,
the full PR body, the project's contribution conventions as actually observed
in that repo, and per-patch caveats. Those files are untracked and listed in
each clone's `.git/info/exclude`, so they cannot be committed into a PR by
accident.

### The four

| # | clone | branch | commits |
| --- | --- | --- | --- |
| 1 | `~/src/ext/go-diskfs` | `fat32-sectors-per-fat` | `1957c46`, `3e4c4dd` |
| 2 | `~/src/ext/go-diskfs` | `fat-volume-label-trim` | `252cab4` |
| 3 | `~/src/ext/mdns` | `close-unicast-conns-on-newserver-error` | `71fb4f4` |
| 4 | `~/src/ext/tailscale` | `filestore-self-heal-corrupt-state` | `7d9f97b` |

Branches 1 and 2 are both off `diskfs/go-diskfs@26c6e3a`; 3 off
`pion/mdns@3a7972c`; 4 off `tailscale/tailscale@0fd2f14d`. The earlier
hand-made branch `fat32-sectors-per-fat-64bit` in the go-diskfs clone is
superseded by branch 1 and can be deleted.

### Test results

- **go-diskfs** — `go build`, `go vet`, `gofmt -l` clean, and `go test` for
  every package on both branches. `filesystem/fat32` (221 s), `fat12`,
  `fat16`, `iso9660`, `squashfs`, `partition/*` and the root package all pass.
  `filesystem/ext4`'s suite exceeds the default 10-minute `go test` timeout on
  this machine and is killed; it is untouched by both patches and provably
  shares no code with them (`go list -deps ./filesystem/ext4` names neither
  `fat12` nor `fat32`). `filesystem/iso9660/statt_windows.go` is unformatted
  on upstream master already and was left alone.
- **pion/mdns** — `go build`, `go vet`, `gofmt -l` clean, `go test ./...`
  passes, and `golangci-lint run ./...` against the repo's own config reports
  **40 issues before the change and 40 after** (all pre-existing, in untouched
  files).
- **tailscale** — `gofmt -l ipn/store/` clean, `go build`/`go vet` on
  `./ipn/store/...`, and `go test ./ipn/store/... ./atomicfile/...
  ./logpolicy/...` all pass. The full suite was not run; it is enormous and
  the change touches one function.

Every patch's test was checked to **fail without the fix**, not merely pass
with it.

### What turned out different from this bean's expectations

**go-diskfs patch 1 is two defects, not one, and they are stacked.** The bean
(via `gosd-e3e3`) framed it as the numerator fix; the task framed it as the
`uint16` truncation. They are two independent bugs in the *same one-line
expression*, both still present on master. They are stacked on one branch
because correcting the numerator can raise `sectorsPerFat` by one, so on its
own it makes the `uint16` truncation bite one sector *earlier* — landing the
numerator fix alone would be a mild regression. Commit 1 (`uint16` → 64-bit,
which is `gosd-8kdm`'s patch, rebased onto current master and with the helper
moved so it stops orphaning `Read`'s doc comment) is independently sendable;
commit 2 must follow it. `PROPOSED-PR.md` says so and offers the maintainer
the split.

The numerator derivation was re-verified from scratch rather than taken on
trust: an independent sweep found **2471 defective sizes** between 1 MiB and
300 GiB and **0** with the patch, matching `gosd-e3e3`. Better, it was proved
end-to-end — real `fat32.Create` on sparse 3/16/64 GiB volumes, BPB read back,
and macOS `fsck_msdos -n` reporting `FAT size too small, 784897 entries won't
fit into 6132 sectors` before and completing cleanly after. **3 GiB is the
smallest defective whole-GiB size**, not 16 GiB as previously assumed.

**go-diskfs patch 2's fix site is not the one this bean sketched.**
`gosd-f83b` proposed special-casing the volume-label entry in
`fat12.parseDirEntries`. That breaks the write path: `SetRootDirLabel`
deliberately re-splits the padded label into `filenameShort = label[:8]` /
`fileExtension = label[8:11]`, and `toBytes` re-pads each field, so storing a
9-byte name in `filenameShort` would round-trip out to the wrong bytes. The
patch instead reassembles in `fat12.FileSystem.Label()` — the one place that
asks for a label — leaving the stored fields and the write path untouched. It
reproduces `gosd-f83b`'s exact observed values (`ABCDEFG H` → `ABCDEFGH`,
`A B C D E` → `A B C DE`).

**pion/mdns: the leak still exists at HEAD, but has moved and grown.** Verified
against `pion/mdns@3a7972c` (2026-08-01). The repo was restructured after
v2.1.0: `Server()` is now a deprecated wrapper (`server.go:586`) and the leak
lives in `NewServer` (`server.go:308`). There is a **sixth** leak site the
v2.1.0 note did not list — the IPv6 listen-address resolve, which drops the
already-open IPv4 conn. Rather than the bean's five explicit `closeUnicastConns`
calls, the patch uses one deferred cleanup gated on a hand-off flag, which
covers every current and future error return. The leak was measured, not just
read: **exactly 2 descriptors per failed attempt**, unreclaimed even with GC
disabled.

**tailscale: half of this bean's premise is false.** The bean says tsnet writes
`tailscaled.state` and `tailscaled.log.conf` "without a write→rename". It does
not. Both go through `atomicfile.WriteFile` — create-temp-in-place → write →
fsync → rename — and have since 2022. This was checked at HEAD *and* at our
pinned `v1.102.2`, which are byte-identical over these files. Further,
`tailscaled.log.conf` **already self-heals**: `logpolicy.New` falls back to a
fresh `Config` when `ConfigFromFile` fails. So the `tailscaled.log.conf` half
of `cmd/gosd-tsfunnel`'s workaround was never needed.

The real, live gap is exactly one branch: `NewFileStore` treats a *zero-byte*
state file as missing but a non-empty *unparseable* one as fatal. That is what
the patch fixes, and it moves the bad file to `<path>.corrupt` rather than
deleting it, which answers the obvious "you are silently discarding a node
identity" objection. **Do not send the bean's original framing** — claiming
tailscale lacks atomic writes would be wrong on the facts and would cost the
report its credibility. `PROPOSED-PR.md` carries the corrected version.

One loose end recorded and deliberately *not* sent: `atomicfile.WriteFile`
fsyncs the file but never the containing directory after the rename, so the
rename is not guaranteed durable across power loss. That is a genuine
second-order gap but it is not diagnosed well enough to assert, and the patch
does not depend on it.

### Blockers before any of these can be sent

- **Tailscale requires an issue first**, and requires a `Fixes #NNN` /
  `Updates #NNN` trailer on the commit. The commit deliberately has no trailer
  yet. File the issue, then amend. Tailscale is DCO (`Signed-off-by`), no CLA.
- **go-diskfs** has no CONTRIBUTING.md and no CLA; `Signed-off-by` is the
  observed house norm (40 of the last 50 commits) and the commits carry one.
- **pion/mdns** uses no sign-off at all (0 of the last 40 commits), so that
  commit has none.
- Commits are authored as `JP Hastings-Spital <jphastings@gmail.com>`, not the
  `jp@byjp.me` in the global git config. Each `PROPOSED-PR.md` gives the
  command to change it.

### Waiting on JP

Nothing here is blocked on further work. **All four are waiting on JP to
review the wording and decide whether and in what order to send them** — and,
for tailscale, to file the issue first. This bean stays in-progress until they
are sent; it is not done when the branches exist, only when the PRs are open.

The last todo — retiring GoSD's local mitigations — stays open and cannot
start until something is merged upstream.
