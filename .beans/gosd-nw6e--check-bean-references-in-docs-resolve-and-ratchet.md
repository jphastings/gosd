---
# gosd-nw6e
title: Check bean references in docs resolve, and ratchet down bare doc paths
status: in-progress
type: task
priority: normal
created_at: 2026-08-17T16:56:52Z
updated_at: 2026-08-17T17:59:07Z
parent: gosd-8pgg
---

Part of epic gosd-8pgg. Stacked on gosd-ihdn (needs `internal/repocheck`).
Independent of the other repocheck children — different files, no conflict.

Two documentation invariants, both currently unchecked. Nothing in the repo
reads a markdown file for any purpose today.

## Part 1 — bean-reference integrity (`internal/repocheck/beanrefs_test.go`)

Every `gosd-xxxx` id appearing in `docs/`, `README.md`, `COMPATIBILITY.md` and
`CLAUDE.md` must resolve to a file under `.beans/` or `.beans/archive/`. All
121 such ids resolve today, so this lands green and stays a pure regression
guard against a renamed or deleted bean orphaning a citation.

### Locked decisions

- Build the valid id set from `os.ReadDir` on `.beans` and `.beans/archive`,
  cutting each filename at `--`. Every tracked bean file matches
  `^gosd-[a-z0-9]{4}--`. (`.beans` is dot-prefixed so the Go tool ignores it,
  but `os.ReadDir` is unaffected.)
- **The obvious regex is wrong.** `\bgosd-[a-z0-9]{4}\b` matches `gosd-data`
  inside `gosd-data-established`, because `-` is a non-word character — `\b`
  does not save you. Go's `regexp` has no lookahead, so match the *maximal*
  token `gosd-[a-z0-9]{4}[-a-z0-9]*` and reject any match longer than 9 chars.
  Then exclude the two known non-bean tokens by name: `gosd-init` (the second
  binary) and `gosd-data` (the partition label / `gosd-data-established`
  marker prefix). Verified: zero false positives, zero false negatives.
- **Do not scan `.beans/` itself.** Beans cross-reference each other and
  archived beans cite superseded ids; that is a different invariant with a
  different noise profile. State the scoping decision in the doc comment.
- **No reverse direction** — most beans aren't referenced from docs. Say so
  explicitly, so the asymmetry reads as a decision rather than an omission.

## Part 2 — doc-link ratchet (`internal/repocheck/doclinks_test.go`)

The convention is that a doc hyperlinks a *descriptive phrase*, never names a
file path in prose. Today ~103 of 141 references violate it, so this ships as a
**ratchet, not a remediation**: per-file violation counts that may not rise.

### Locked decisions

- Two violation classes, both counted outside fenced code blocks: a backticked
  repo-doc path with no link, and a link whose text *is* its target path
  (``[`docs/runtime.md`](docs/runtime.md)`` — a link whose text is its own URL).
- `maxViolations map[string]int` seeded with today's counts; fail when a file's
  count rises. Fixing violations and lowering the numbers is opportunistic
  follow-up work, not this bean's job.
- **`CLAUDE.md` is excluded outright** — 16 bare paths and zero markdown links.
  It is a machine-oriented index, not prose, and that is deliberate.
- Keep the check purely lexical (does this token *look* like a repo doc path).
  A "the path must exist on disk" check false-positives heavily: docs cite
  generic filenames (`main.go`, `gosd-kernel.toml`) and paths inside *other*
  repos (rpi-imager's `doc/schema-notes.md`).
- Point the failure message at COMPATIBILITY.md as the exemplar — 14
  descriptive links, 1 bare.

## Todo

- [x] `internal/repocheck/beanrefs_test.go`
- [x] Verify it bites: add `gosd-zzzz` to a doc
- [x] Confirm `gosd-init` / `gosd-data` / `gosd-data-established` do not trip it
- [x] `internal/repocheck/doclinks_test.go` with counts seeded from the current tree
- [x] Verify it bites: add a bare backticked doc path to a doc
- [x] Add a short CLAUDE.md line under Code conventions recording the doc-link convention and that it is ratcheted
- [x] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

No changeset — internal only, use the `no release notes` label.

## Summary of Changes

Two test files in `internal/repocheck` (in-package `package repocheck`, so the
sibling checks landing in the same directory stay independent), plus one
CLAUDE.md convention line. No production code.

**beanrefs_test.go — TestDocsCiteOnlyKnownBeans.** Scans `docs/**/*.md`,
`README.md`, `COMPATIBILITY.md` and `CLAUDE.md`; every `gosd-xxxx` token must
resolve to a filename prefix under `.beans/` or `.beans/archive/`. The maximal
token `gosd-[a-z0-9]{4}[-a-z0-9]*` is matched and anything longer than 9 bytes
discarded, then `gosd-init` and `gosd-data` are skipped by name. Verified on
today's tree: 103 distinct 9-byte tokens appear, 101 are beans, and the two
that are not are exactly the pair excluded by name — so it lands green as a
pure regression guard, as expected. Probed by appending `gosd-zzzz` to
COMPATIBILITY.md (one failure, correctly located) alongside `gosd-init`,
`gosd-data` and `gosd-data-established` on the same line (none tripped).

**doclinks_test.go — TestDocPathsAreLinkedNotNamed.** Ratchet only. Counts,
per file and outside fenced code blocks, an inline-code span that is nothing
but a markdown path, and a link whose text is a path rather than a phrase.
Seeded totals: 83 violations across 13 files, against 41 already-compliant
descriptive links (124 references in all). Per file: README.md 17,
docs/runtime.md 13, docs/publishing.md 11, docs/externals.md 7,
docs/provisioning-formats.md 7, docs/design/ab-updates.md 6,
docs/design/upgrade-path.md 6, docs/ingress.md 5, docs/custom-kernels.md 4,
docs/image-injection.md 4, COMPATIBILITY.md 1, docs/flashing.md 1,
docs/sound.md 1. A rise fails; a drop only logs a nudge to lower the entry, and
a key naming a file that is no longer scanned fails so a rename cannot silently
drop a budget.

### Two deviations from the locked decisions, both widening

- **Class 2 is "the link's text is a path", not "the link's text equals its target".** The literal reading is nearly vacuous here: the dominant form in
  this repo is ``[`docs/publishing.md`](publishing.md)`` — text names the
  repo-relative path, target is the sibling-relative one — plus
  `[docs/flashing.md](https://github.com/…/docs/flashing.md)`, whose target is
  a URL. Only a handful are literally equal, so the narrow rule would have
  ratcheted almost nothing.
- **The .explain.md sidecars and LAST_FATAL_ERROR.md are excluded.** They are
  lexically doc paths but name files on a flashed card, so there is nothing
  for prose to have linked to instead. Left in, `docs/config.md` alone would
  carry a double-figure budget that legitimate new prose about the config tree
  could not stay under. The check is still purely lexical — no filesystem
  probe — so rpi-imager's `doc/schema-notes.md` is counted, as the bean
  intends.

### Contradicted assumption

The bean's "~103 of 141" is not reproducible under any reading I tried; the
measured figure is 83 of 124. The gap is the two exclusions above (the
`explain.md` family accounts for most of it) and CLAUDE.md's own 16 bare
paths, which the bean excludes from the check but may have counted in the
estimate. Nothing about the shape of the invariant changes.

### Not done, deliberately

No prose was fixed. The 83 stay as they are.
