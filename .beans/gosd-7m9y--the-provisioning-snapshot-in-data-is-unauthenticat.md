---
# gosd-7m9y
title: The provisioning snapshot in /data is unauthenticated, so a planted one survives reflash and re-provisions the device
status: in-progress
type: bug
priority: normal
created_at: 2026-08-12T04:15:07Z
updated_at: 2026-08-20T04:39:58Z
---

**Severity: High.** Defeats reflashing — the owner's primary and most drastic
remediation — and can silently re-point a device's WiFi and public tunnel.

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

1. **Do not silently restore secrets.** Restoring an `[ingress.*]` token or
   authkey from unauthenticated storage is the highest-value part to close.
   Either drop `[ingress.*]` from the restore set, or gate it behind an
   explicit opt-in in the freshly-flashed gosd.toml.
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
non-credential setting still survives a reflash.** What shipped is a
narrowing of the blast radius, not an authenticity control, and the bean's
own guidance ("be honest about what is achievable", "do not file this as
'add HMAC'") is what it follows.

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

1. **Bearer credentials are never kept, and so never restored**
   (`configtree.IsCredential`: `ingress/cloudflared/token`,
   `ingress/tailscale-funnel/authkey`). This is the bean's option 1, taken
   in its "drop from the restore set" form rather than the opt-in form: an
   opt-in would have meant a new config-tree key and its whole apparatus,
   when the recovery path already exists and is the same act that set the
   value originally — type it onto the card. The distinction being drawn is
   real and worth stating: every other setting says what the device should
   *do*; a token or authkey *is* the authorisation to reach it, from
   anywhere. The store never holds one, and a copy an earlier release left
   there is deleted on the next boot.
   - **Adversarial finding, caught in review of my own patch:** the first
     version matched paths case-sensitively. Both partitions can be FAT,
     where `Token` and `token` are one file — so planting the value under a
     different capitalization would have been restored onto the card and
     landed in the very file the device reads. `IsCredential` folds case,
     as `checkCollisions` already does for the same reason, and a test
     pins it.
   - A `configtree` test walks the shipped defaults tree and fails on any
     credential-shaped name (`token`, `authkey`, `secret`, `password`,
     `passphrase`, …) that is neither refused nor deliberately exempted
     with a written reason, so the next ingress agent cannot ship a
     restorable token by omission. `wifi/passphrase` is the one recorded
     exemption: refusing it alone would leave a device trying to join its
     own network with no key, or joining an attacker's open one.
2. **Every restored value is validated by the gates the card's own path
   uses** — see `gosd-39da` for the field-by-field audit. The structural
   property that makes this true, and that is now stated in the package
   doc, is that a restored value is written onto the card and read back out
   of the tree: there is no restore path that reaches a sink without
   passing what a hand-edited card passes. Two gaps closed on the way:
   `configtree.ValidEnvName` applied at runtime, and
   `configtree.PlausibleValue` refusing a NUL (which would have made
   `execve(2)` fail, stopping `/app` on every boot — a sticky denial of
   service surviving the reflash performed to fix it).
3. **A restore announces itself**, once before the per-setting lines,
   naming the partition it came from and that gosd cannot tell who wrote
   it.
4. **Documented as a trust boundary** in the config guide ("A reflash is
   not a factory reset"), the upgrade-path spike (§3a, which weighed this
   purely as an availability problem and never asked the other question),
   and the package doc.
5. **Package doc corrected** — it previously stated the digest was not an
   authenticity control in one sentence at the end; it now leads with the
   integrity-vs-authenticity distinction and enumerates why no key exists.

### Crash-ordering argument

The store's existing commit discipline is unchanged: an entry commits as
value → sync → digest → sync, a delete removes the digest *first*, and the
image identity is written last, after both phases, because it is the record
that ends a restore window. This PR adds one new on-disk act — deleting a
stored credential (and a stored NUL-carrying value) — and it is
deliberately placed where its durability is *not* load-bearing. The purge
happens in `load`, before the restore phase reads anything, and the entry
is excluded from the in-memory `stored` map whether or not the delete
reached the disk; so a power cut during the purge cannot cause a restore on
that boot or any other. The delete itself reuses `deleteEntry`, which
removes the digest before the value and fsyncs each directory: a crash
between the two leaves a value with no digest, which reads as torn and is
dropped by the next `load` — the same end state the delete was reaching
for, so the purge is convergent rather than merely retried. A purge that
fails outright is logged and does not set `readable=false`, which is
correct and worth being explicit about: `readable` gates deletions and the
identity stamp for settings the store *keeps*, and a credential is by
definition not one of those, so a stale credential file lingering inertly
on `/data` must not also cost the device a boot's worth of reconciliation
for every other setting. Nothing here is gated on a filesystem probe: the
credential decision is made from the entry's path, and the value decision
from bytes that hashed to their own digest.

### What remains true, and is now written down

A plain reflash is not a factory reset. Anything with write access to
`/data` — someone who has had the card, or the app itself, running as root
with `/data` as its storage — can still put a *non-credential* setting onto
a freshly flashed card. Clearing or reformatting the data partition is the
operation that resets a device, and on an ext4 `/data` that needs a Linux
host.

### Deferred (worth a follow-up bean if wanted)

- **Hand a refused credential back on the card** as an inert
  `token.unused`-style file rather than deleting it, so an owner upgrading
  from an earlier release can retrieve their own token once — and so a
  *planted* one becomes visible evidence rather than a silent deletion.
  Not done here: it needs a third reserved suffix with build-side refusal,
  and writing secret material onto the card to solve a convenience problem
  deserves its own decision.
- **A "reset me" affordance** — a file on the boot partition that tells the
  next boot to clear the store — would give owners the remediation a
  reflash currently implies but does not perform, without needing a key.
