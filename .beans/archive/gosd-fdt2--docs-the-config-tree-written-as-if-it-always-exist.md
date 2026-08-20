---
# gosd-fdt2
title: 'Docs: the config tree, written as if it always existed'
status: completed
type: task
priority: normal
created_at: 2026-08-13T15:40:14Z
updated_at: 2026-08-13T20:21:06Z
parent: gosd-rw6n
blocked_by:
    - gosd-87ip
---

Documentation for epic gosd-rw6n. Per JP: **no mention of the previous format
anywhere** — every page is written as if the config tree always existed. No
"formerly gosd.toml", no migration notes, no legacy sections.

## Todos

- [x] New `docs/config.md` (replaces `docs/gosd.toml.md`, which is deleted):
      the tree layout, editing values on a mounted card, explain.md files,
      reserved names and the FAT-junk rules, padding/reservation, how the
      Imager wizard's values land in the tree, the store/restore behaviour
      (`.new`, `.unused`, revert-is-a-default), and the secrets note
      (plaintext on the boot FAT; a copy lives in /data via the store, so a
      reflash is not a credentials wipe — only clearing /data is)
- [x] `docs/image-injection.md`: the manifest `config` array and the TS
      `config` option; the placeholder contract unchanged
- [x] `docs/runtime.md`: the app-environment section and the
      persistence/reflash section rewritten around the tree and the store
- [x] Sweep every other reference: `docs/ingress.md`, `docs/publishing.md`,
      `docs/flashing.md`, `docs/provisioning-formats.md`, README,
      COMPATIBILITY.md
- [x] `docs/releases/UNRELEASED.md`: the standing call-out rewritten to
      describe the tree only (the current text describes `--env-placeholder`,
      which never shipped)
- [x] CLAUDE.md: the "Naming surfaces" and provisioning locked decisions
      updated (gosd.toml -> the config tree), and the epic recorded
- [x] npm README already updated in the build-side bean; verify no stale
      references remain in `js/`

## Summary of Changes

Every doc that described `gosd.toml` now describes the config tree, written
as if it always existed:

- **New `docs/config.md`** replaces the deleted `docs/gosd.toml.md`: tree
  layout, editing a value on a mounted card, `explain.md` sidecars, the
  reserved-name/FAT-junk rules (verified against `internal/configtree`'s
  `checkName`/`IgnoredName`), padding-is-the-reservation, how an Imager
  wizard's answers land in the tree (verified against `consumeCloudInit`),
  and the config store's restore/`.new`/`.unused`/revert-is-default
  behaviour (verified against `cmd/gosd-init/internal/configstore`'s
  package doc and `Reconcile`/`restorePlan`/`persist`).
- **`docs/image-injection.md`**: swept the handful of `gosd.toml` mentions
  (the boot sentinel, placeholder-collision rules, Imager overwrite
  description) — the manifest `config` array section was already accurate
  against `internal/inject`, no change needed there.
- **`docs/runtime.md`**: rewrote "App environment variables" around
  `config/env/`, the WiFi/ingress precedence bullets, the `flush`
  mount-option cross-reference, the Provisioning section (cloud-init as a
  consumed seed, not a competing runtime source), and replaced "The
  provisioning snapshot" with "Keeping settings across a reflash: the
  config store", rewritten from `configstore`'s actual per-file mechanics
  rather than the old whole-document snapshot.
- **Swept**: `docs/ingress.md` (rewritten throughout — settings paths,
  runbooks, and the troubleshooting tables' log lines updated to match the
  literal strings in `cloudflared/mode.go`/`tsfunnel/mode.go` verbatim),
  `docs/publishing.md`, `docs/flashing.md` (end-user troubleshooting now
  walks the `config/wifi/` folder), `docs/provisioning-formats.md`
  (rewrote the "Precedence" section — cloud-init is a seed, not a
  three-way runtime precedence chain, matching `consumeCloudInit`),
  `README.md`, `COMPATIBILITY.md`.
- **Also swept** (found while grepping, not originally itemized):
  `docs/crash-reports.md`, `docs/custom-kernels.md`, `docs/externals.md`,
  the three example READMEs (`chime`, `sattrack`, `usbwebsite`), and
  `docs/design/upgrade-path.md` / `docs/design/ab-updates.md` (design-spike
  decision records — kept their historical framing/dates but updated the
  mechanism descriptions and terminology so a reader following a live
  cross-link from `runtime.md`/`publishing.md` doesn't land on stale
  `gosd.toml` prose; see the report back to the parent agent for the full
  reasoning on why these were in scope while bean bodies and the
  `internal/provision/testdata` fixture capture notes were left alone).
- **`docs/releases/UNRELEASED.md`**: already accurate (written by the
  build-side bean); verified against the actual shipped mechanism, no
  changes needed.
- **CLAUDE.md**: the three requested edits (Naming surfaces,
  End-user-flashing-path consequence sentence, new locked-decision bullet
  recording epic gosd-rw6n) plus a small number of other sentences that
  presented `gosd.toml` as current (Default hostname's dead `--hostname`
  flag, the ext4 `--data-filesystem` bullet, the vfat-flush bullet) — left
  every historical bean reference untouched.
- **`js/`**: no stale references in the npm README (already correct); found
  and fixed one genuinely stale, half-edited comment in
  `substitute.test.ts` (a leftover pre-config-tree line a previous bean's
  edit hadn't fully removed).
- **`internal/configtree/defaults/ingress/cloudflared/explain.md`**: added
  JP's fuller cloudflared setup walkthrough (account/domain/CLI
  prerequisites, login → create tunnel → print token → route DNS) — this
  changes on-card bytes of the shipped defaults tree, flagged in the PR
  body for JP to veto. `token.explain.md`/`hostname.explain.md`/
  `port.explain.md` were reviewed and left as already-adequate.

All Go gates (`go test`, `go vet`, `gofmt`, `golangci-lint` both native and
`GOOS=linux`) and all JS gates (format/lint/typecheck/build/test/
test:integration) pass.
