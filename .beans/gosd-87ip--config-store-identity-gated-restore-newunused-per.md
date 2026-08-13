---
# gosd-87ip
title: 'Config store: identity-gated restore, .new/.unused, per-file persistence'
status: todo
type: feature
priority: normal
created_at: 2026-08-13T15:39:58Z
updated_at: 2026-08-13T15:40:23Z
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

- [ ] Store layout under /data (mirror of the tree + per-entry digest
      sidecar + store identity record); every entry commits
      write -> sync -> digest -> sync; a torn entry is dropped, never trusted
- [ ] Restore phase with the per-file rules and `.new` writing
- [ ] Persist phase with upsert and delete-on-revert
- [ ] Orphan handling (`.unused` on the card, env/ exemption)
- [ ] An explicit crash-ordering argument in the package doc, and an
      adversarial review pass BEFORE requesting JP's review (repo rule for
      anything that commits on-disk state; probe-only gates have been
      rejected before — gosd-lirl)
- [ ] Fake-driven unit tests (macOS-safe) pinning: restore-only-on-identity-
      change, injected-card-beats-store, revert-deletes-entry,
      orphan-to-.unused, torn-entry-dropped, `.new` only for non-empty
      differing defaults
- [ ] Extend the qemu reflash re-adoption CI job end-to-end: edit -> reflash
      -> restored; inject -> reflash -> injection wins; revert -> reflash ->
      default stands
- [ ] Quality gates: `go test ./...`, `go vet ./...`, `gofmt -l .`,
      `golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`
