---
# gosd-87ip
title: 'Config store: identity-gated restore, .new/.unused, per-file persistence'
status: completed
type: feature
priority: normal
created_at: 2026-08-13T15:39:58Z
updated_at: 2026-08-13T18:21:04Z
parent: gosd-rw6n
blocked_by:
    - gosd-ypkv
---

The persistence half of epic gosd-rw6n (which holds all locked decisions):
the per-file store on /data that replaces the deleted provsnapshot, carrying
customer configuration across reflashes.

The rules, restated tersely (the epic has the full rationale):

- Presence in the store IS intent; no old-default hashes.
- Store records the image identity it was last written under; the restore
  phase runs ONLY when the running identity differs.
- Restore, per file: card ≠ baked -> card wins. Card == baked + store entry
  -> restore the stored value; write the baked value as `<name>.new` when
  non-empty and different (overwrite an existing `.new`).
- Persist, every boot, after cloud-init: card ≠ baked -> upsert. Card ==
  baked + store entry -> DELETE the entry (a hand-revert is a default; no
  `.new` marks it).
- Orphans outside `env/`: write to card as `<name>.unused`, drop from store.
  `env/` restores normally (customer namespace).
- Explanations never persisted or restored.

## Todos

- [x] Store layout under /data (mirror of the tree + per-entry digest
      sidecar + store identity record); every entry commits
      write -> sync -> digest -> sync; a torn entry is dropped, never trusted
- [x] Restore phase with the per-file rules and `.new` writing
- [x] Persist phase with upsert and delete-on-revert
- [x] Orphan handling (`.unused` on the card, env/ exemption)
- [x] An explicit crash-ordering argument in the package doc, and an
      adversarial review pass BEFORE requesting JP's review (repo rule for
      anything that commits on-disk state; probe-only gates have been
      rejected before — gosd-lirl)
- [x] Fake-driven unit tests (macOS-safe) pinning: restore-only-on-identity-
      change, injected-card-beats-store, revert-deletes-entry,
      orphan-to-.unused, torn-entry-dropped, `.new` only for non-empty
      differing defaults
- [x] Extend the qemu reflash re-adoption CI job end-to-end: edit -> reflash
      -> restored; inject -> reflash -> injection wins; revert -> reflash ->
      default stands
- [x] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
      `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`

## Summary of Changes

`cmd/gosd-init/internal/configstore` keeps a copy of a device's own settings
on the data partition and puts them back after a re-flash.

**Layout** (`/data/.gosd/config`, `configstore.Dir`), a mirror of the card's
tree:

    values/<tree path>    the value, exactly as the card reads it
    digests/<tree path>   hex SHA-256 of that value file, written second
    identity              the image identity the store last reconciled with

Parallel `values/`+`digests/` trees rather than a `<name>.sha256` sidecar,
because a setting's name may contain periods (`google-service-account.json`),
so any suffix convention is a name a real setting could have.

**Boot ordering** (`boot.Options.ConfigStoreDir`, empty disables): the
reconcile sits immediately after `mountData` — the earliest point /data
exists, and the last point that is any use, since everything below it acts on
a setting. `consumeCloudInit` was NOT moved: it already runs before this, so
the locked "persist after cloud-init consumption" order holds, and a wizard's
answers reach the card first and therefore win the restore as an ordinary
card edit. A restored `hostname` is re-resolved and re-applied on the spot
(`Result.Restored`), before /app starts.

**One accepted consequence:** `data_flush` is read before `mountData` because
it decides how /data is mounted, so a restored `data_flush` takes effect from
the boot after the re-flash. That costs one boot of ordinary writeback and
nothing else — durability never comes from that option (gosd-9m1k).

**"card == baked" is byte-for-byte**, over the file as the card holds it,
padding included: config.json carries a digest per value file, never a copy of
the value, so it is the only comparison available — and the right one. A file
byte-identical to the shipped one is the shipped one; a file somebody retyped,
even to the same words, is one they chose. Re-flashing the same image is the
byte-identical revert, which is why it forgets the settings it had; clearing a
setting by hand leaves an empty file, which is kept as the wish for it to be
unset.

**Crash ordering** (full argument in the package doc): value → sync → digest →
sync, digest-last, so an entry that doesn't hash to its digest is torn and is
dropped, never trusted. Deletes remove the digest first, so a crash mid-delete
leaves a torn entry — the same outcome the delete wanted. Removals fsync their
directory. The identity record is written LAST, only after both phases
completed without error: written any earlier it would vouch for entries not yet
on the card, and the next boot would skip the restore, find every un-restored
setting equal to its default, and delete exactly the entries with work left.
Restore is per-file idempotent, so a stale identity only costs a repeat.

## Adversarial review pass

Done inline before review; two real findings, both fixed:

1. **A transient read error destroyed intent.** A value that wouldn't read
   (I/O error) was indistinguishable from one that was never fully written, so
   it was deleted — and, worse, the boot still stamped the identity, so the
   NEXT boot saw a matching identity, read a freshly flashed card as "every
   setting was put back by hand", and deleted the rest. Now three states are
   distinguished (`entryOK`/`entryTorn`/`entryUnreadable`): only a value that
   reads back and disagrees with its digest is torn; anything unreadable —
   including an unreadable identity file or a walk error — leaves the whole
   boot un-reconciled: nothing deleted, no identity recorded, retried next
   boot. Pinned by `TestAKeptSettingThatWontReadIsLeftAloneRatherThanWrittenOff`.
2. **A stored path the card still carries was handed back AND re-recorded.**
   A non-`env/` path absent from the baked tree but present on the card (made
   by hand before first boot) got both a `.unused` file and an immediate
   re-upsert from the card. The "card carries it" case now precedes the orphan
   case, so an orphan means the image doesn't have it *and* the card doesn't
   either.

Checked and confirmed safe: no probe-only gate (adoption requires a digest
match, never a file's existence); no reliance on POSIX rename atomicity (every
write is `durable.WriteFile`, and torn-ness is proved by digest, not by the
rename having landed); the identity can never precede what it vouches for
(written last, gated on `reconciled && kept`), and every failure direction is
"restore again" rather than "skip the restore"; deletions are additionally
gated on the restore phase having reached the card, so a restore that could
not be written can't read as a revert on the boot after; values are never
logged, only paths; nothing here is fatal — a wedged store costs a
reconciliation, never a boot.

## CI

`qemu-expand-data` now proves the whole story across five boots on the real
boot path, with mtools making the card edits between them (the same file a
person edits in a card reader, and the same bytes a provisioning tool
overwrites in a downloaded .img). A second image built with
`--config-dir .github/testdata/reflash-config` supplies the differing identity
a restore needs — a config tree is part of the hashed boot payload:

- boot 1-2: an edited `config/hostname` and a customer-made
  `config/env/GREETING` are what the app runs with, and are kept
- boot 3 (re-flash to the other image): `hostname` is restored over that
  image's own non-empty default, `config/hostname.new` holds
  `reflashed-default`, and an injected `GREETING` beats the kept copy
- boot 4 (same image re-flashed): the byte-identical revert; the kept
  hostname is forgotten and the image's own value stands
- boot 5 (back to the first image): the forgotten hostname stays forgotten
  while the env var — which no image ships, so no image can retire it — is
  still restored
