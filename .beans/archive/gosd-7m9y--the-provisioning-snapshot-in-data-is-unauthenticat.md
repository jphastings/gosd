---
# gosd-7m9y
title: The provisioning snapshot in /data is unauthenticated, so a planted one survives reflash and re-provisions the device
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:15:07Z
updated_at: 2026-08-20T08:43:12Z
---

**Severity: High.** Defeats reflashing — the owner's primary and most drastic
remediation — and can silently re-point a device's WiFi and public tunnel.

> **Decided 2026-08-20, by JP, and it overrides this bean's own first
> recommendation.** See "The decision" below before acting on the Fix
> section: **credentials are kept and restored like every other setting.**

## Verified

`cmd/gosd-init/internal/provsnapshot/provsnapshot.go:676`:

```go
if got := digest(tomlData); got != meta.GosdTomlSHA256 {
```

A plain, **unkeyed** SHA-256 of the snapshot's own gosd.toml, stored in
`snapshot.json` right beside it. Its documented purpose (package doc,
`:26-31`) is detecting a **torn write**. It cannot detect a forged one:
anyone who can write both files computes a consistent pair offline.

`heal()` (`:312-330`) skips the restore only when
`in.Identity == snap.Identity`. It therefore fires **precisely on a
reflash** to any different or updated image.

`planRestore` then restores hostname, WiFi SSID + passphrase, the whole
`[ingress.cloudflared]` / `[ingress.tailscale-funnel]` table (token and
authkey included), and `[env]` values, writing them back to the boot
partition (`heal`, `:361`) and applying them to the running boot.

## Who can write /data

Two realistic actors, and the second matters most:

1. Physical access to the card, or to the board over USB mass storage — the
   shipped `examples/usbwebsite` shares the data partition read-write (see
   the USB mass-storage bean; the two chain).
2. **A compromised `/app`.** It runs as root and `/data` is its own storage.
   So an app compromise can plant a snapshot and persist its configuration
   across the reflash the owner performs to clean up.

## Attack

Attacker plants `/data/.gosd/provision-snapshot/{gosd.toml,snapshot.json}`
with `Identity` set to the currently-running image's, a correctly-computed
digest, and `Effective.Wifi = {SSID: "evil-ap", ...}` or an
attacker-controlled `[ingress.cloudflared]` token.

Months later the owner reflashes with a new image, believing that resets
provisioning. `heal()` sees a different identity, finds nothing fresher on
the new card, and restores the attacker's WiFi network or tunnel token. The
device joins the attacker's network, or opens a tunnel the attacker
controls, having just been "factory reset". `/data` is untouched by an
Imager reflash, and cannot be cleared from a macOS host at all when it is
ext4.

## Fix — be honest about what is achievable

A keyed MAC has nowhere to keep its key: the boot partition is wiped by the
reflash this must survive, `/data` is what the attacker reads, and these
boards have no TPM or secure element. So do not file this as "add HMAC".
The practical measures, in order of value:

1. ~~**Do not silently restore secrets.** Restoring an `[ingress.*]` token or
   authkey from unauthenticated storage is the highest-value part to close.
   Either drop `[ingress.*]` from the restore set, or gate it behind an
   explicit opt-in in the freshly-flashed gosd.toml.~~ **Overruled — see
   "The decision" below. Credentials are kept and restored.**
2. **Validate everything restored**, to the same standard as the on-card
   path — the hostname gate is missing today (sibling bean).
3. **Log loudly** what was restored and from where, so a restore is visible
   in the console rather than silent.
4. **Document that `/data` is a trust boundary** and that a plain reflash
   does not clear provisioning. Users reasonably assume the opposite, and
   `docs/design/upgrade-path.md`'s adoption-gate language encourages that
   assumption.

## Todos

- [x] Decide and record whether `[ingress.*]` is restorable at all from an unauthenticated snapshot
- [x] Validate every restored field with the same gates the on-card path uses
- [x] Log each restored key at boot
- [x] Document /data as a trust boundary that survives reflash, in docs/design/upgrade-path.md and the provsnapshot package doc
- [x] Correct the package doc, which presents the digest as an integrity control without stating it is not an authenticity control

## The decision: credentials are kept (JP, 2026-08-20)

The first version of this PR removed `ingress/cloudflared/token` and
`ingress/tailscale-funnel/authkey` from what the config store keeps, and
purged any copy already on `/data`. **JP rejected that**, and the reasoning
is worth recording in full because it is the kind of change that looks like
pure upside from inside a security bean:

- **A reflash must keep ALL of a device's settings, credentials included.**
  That is the store's locked purpose — CLAUDE.md states it as "puts the
  settings somebody put on the card back onto the newly flashed one" — and
  narrowing it is a product change this bean was never authorised to make.
- **The exclusion would have forfeited the feature while keeping the whole
  of the risk.** Every other setting still survives a reflash, so a planted
  hostname, SSID or env var still reaches a freshly flashed card. What the
  exclusion bought was not "the attack stops working"; it was "the attack
  works, and additionally the owner's own tunnel stops working after every
  reflash". The values hardest to retype are exactly the ones an owner most
  needs back.
- **The residual risk is ACCEPTED, not mitigated.** Anything able to write
  `/data` — someone who has had the card, or the app itself, which runs as
  root with `/data` as its storage — can put a setting onto a freshly
  flashed card. That is now written down for developers (the `configstore`
  package doc, the upgrade-path spike §3a) and for users (docs/config.md,
  "A reflash is not a factory reset") rather than partially papered over.
- **The compensating control is a real reset path**, not a smaller restore
  set: bean `gosd-df24` designs a factory reset an owner triggers from the
  **boot** partition — the only partition editable from macOS or Windows,
  which is precisely why today's documented remedy ("clear `/data`") is
  unavailable to most owners on an ext4 data partition.

## Summary of Changes

Shipped with `gosd-39da` in one PR.

**Read this first: the architecture moved under the bean.** `provsnapshot`
and its `gosd.toml` no longer exist — epic `gosd-rw6n` replaced them with
the per-file config tree and `cmd/gosd-init/internal/configstore`. Every
finding transferred intact: the store keeps one value per setting on
`/data`, each vouched for by an unkeyed SHA-256 written beside it, and
restores them onto the card on the first boot under a *different* image
identity — i.e. precisely on a reflash. Only the file names changed.

### The honest answer on authenticity: we did NOT achieve it

Stated plainly, because it would be easy to read this PR as having fixed
the bean's headline: **the store is still unauthenticated, and a planted
setting still survives a reflash.** What shipped is hardening of the
*restore path* plus an honest, published account of the limitation — which
is what the bean itself asks for ("be honest about what is achievable", "do
not file this as 'add HMAC'").

The digest was never the problem to fix and adding a keyed one is not
available. Two different properties:

- **Integrity — what the digest gives.** The value was written
  *completely*. It is what makes a torn write, or a half-linked file a FAT
  rename left behind, distinguishable from a value.
- **Authenticity — what is missing.** *Who* wrote it. An unkeyed SHA-256
  stored beside the bytes it covers proves nothing here: anyone who can
  write one file writes both.

A keyed MAC needs a key the verifier reads and the attacker cannot, and
there is nowhere on these boards to put one. Each candidate fails for its
own reason, and the fourth is the one that settles it:

1. **Boot partition** — erased by the very reflash the store exists to
   survive. Gone at the moment it would be used.
2. **`/data`** — the partition the attacker is writing. A key beside the
   values vouches for the attacker's values too.
3. **TPM / secure element** — no board GoSD supports has one.
4. **Hardware-derived (SoC serial, card CID)** — would stop the actor who
   only ever holds the card, and *not* the actor the bean names as
   mattering most: a compromised `/app` runs as root, reads whatever the
   derivation reads, and computes the MAC itself. Defending against the
   lesser actor while looking like a defence against the greater one is
   worse than not defending at all.

**Binding the store to image identity — the mitigation the task suggested
considering — is not available here either, and for an instructive reason:
it is inverted.** The store restores *because* the identity differs. A
snapshot "from a different image" is not the attack signature; it is the
signature of the feature working. Any check of that shape would disable
restore entirely rather than authenticate it.

Recorded as a **non-goal with a reason** rather than an open task, in the
package doc, the design doc and the user-facing guide.

### What did ship

1. **The restore is not a privileged path.** Every restored value is
   written onto the card and read back out of the tree, so it passes the
   identical gates a hand-edited card passes — restoring a value can do
   nothing that typing it onto the card couldn't. That structural property
   is what closes `gosd-39da` (see it for the field-by-field audit), and it
   is now stated in the package doc and pinned by an end-to-end test rather
   than left as a property of the current shape. Two gaps closed on the
   way, both reachable from the store *and* from a hand-edited card:
   - `configtree.ValidEnvName` is now applied at runtime in
     `boot.mergeUserEnv`, holding a `config/env/` name to the same rule the
     build enforces.
   - `configtree.PlausibleValue` refuses a NUL, in `cardconfig` and in the
     store. A NUL makes `execve(2)` fail with EINVAL, so one planted here
     would stop `/app` starting on every boot — a sticky denial of service
     surviving the reflash performed to fix it. Deliberately no stricter: a
     multi-line value pasted into `config/env/` is legitimate.
2. **A restore announces itself**, once before the per-setting lines,
   naming the partition it came from, how many settings it put back, and
   that gosd cannot tell who wrote them.
3. **Documented as a trust boundary** in the config guide ("A reflash is
   not a factory reset"), the upgrade-path spike (§3a, which weighed this
   purely as an availability problem and never asked the other question),
   and the package doc.
4. **Package doc corrected** — it previously stated the digest was not an
   authenticity control in one sentence at the end; it now leads with the
   integrity-vs-authenticity distinction, enumerates why no key exists, and
   records that keeping less was considered and rejected.
5. **A test pins the decision positively** — that a tunnel credential IS
   kept and restored like any other setting — so a future agent reading the
   trust-boundary section doesn't reintroduce the carve-out as an obvious
   improvement.

### Crash-ordering argument

The store's existing commit discipline is unchanged and this PR adds no new
on-disk act beyond one the store already performed: an entry commits as
value → sync → digest → sync, a delete removes the digest *first*, and the
image identity is written last, after both phases, because it is the record
that ends a restore window. The one behavioural addition is that a stored
value carrying a NUL is now classified `entryNotASetting` and deleted, and
its durability is deliberately not load-bearing: the drop happens in
`load`, before the restore phase reads anything, and the entry is excluded
from the in-memory `stored` map whether or not the delete reached the disk,
so a power cut during it cannot cause a restore on that boot or any other.
The delete reuses `deleteEntry`, which removes the digest before the value
and fsyncs each directory: a crash between the two leaves a value with no
digest, which reads as torn and is dropped by the next `load` — the same
end state, so the drop is convergent rather than merely retried. Nothing
here is gated on a filesystem probe: the decision is made from bytes that
hashed to their own digest.

`entryNotASetting` is also kept distinct from `entryTorn` on purpose. Torn
means a write was interrupted; "this could never be a setting" means
nothing was interrupted at all, and saying "wasn't finished being written"
about a device node or a NUL would send somebody looking for a power cut
that never happened.

### What remains true, and is now written down

A plain reflash is not a factory reset. Anything with write access to
`/data` can still put a setting onto a freshly flashed card, credentials
included. Clearing or reformatting the data partition is the operation that
resets a device, and on an ext4 `/data` that needs a Linux host — which is
the gap bean `gosd-df24` closes.

### Why this bean is completed rather than left open

Its headline attack still works, so "completed" deserves justifying. Every
todo it carries is now answered, including the first — which asked for a
**decision** about `[ingress.*]`, and got one, at the product level, from
JP. Authenticity is recorded as a non-goal with the reason, not deferred.
The remaining work is not this bug: it is a distinct, designed feature with
its own bean (`gosd-df24`). Leaving this open would create a backlog item
that no code in this architecture can ever close, and would hide the fact
that the decision has been made. The residual risk is accepted, documented
in three places, and tracked forward.

### Deferred (worth a follow-up bean if wanted)

- **A "reset me" affordance** — now a bean in its own right, `gosd-df24`,
  and the compensating control for the risk accepted above.
