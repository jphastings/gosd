---
# gosd-nw6e
title: Check bean references in docs resolve, and ratchet down bare doc paths
status: todo
type: task
priority: normal
created_at: 2026-08-17T16:56:52Z
updated_at: 2026-08-17T16:57:48Z
parent: gosd-8pgg
blocked_by:
    - gosd-ihdn
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

- [ ] `internal/repocheck/beanrefs_test.go`
- [ ] Verify it bites: add `gosd-zzzz` to a doc
- [ ] Confirm `gosd-init` / `gosd-data` / `gosd-data-established` do not trip it
- [ ] `internal/repocheck/doclinks_test.go` with counts seeded from the current tree
- [ ] Verify it bites: add a bare backticked doc path to a doc
- [ ] Add a short CLAUDE.md line under Code conventions recording the doc-link convention and that it is ratcheted
- [ ] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

No changeset — internal only, use the `no release notes` label.
