---
# gosd-cfnh
title: Add a .golangci.yml whose only job is fencing go-diskfs to its three owners
status: completed
type: task
priority: normal
created_at: 2026-08-17T16:56:12Z
updated_at: 2026-08-17T17:20:47Z
parent: gosd-8pgg
---

Part of epic gosd-8pgg. Independent of the other children — branch from main.

There is no `.golangci.yml` anywhere in the repo. CI runs golangci-lint v2.12.2
with zero config, i.e. the v2 default set (errcheck, govet, ineffassign,
staticcheck, unused). CLAUDE.md's "FAT32 work goes through `internal/diskfmt`'s
wrappers, never go-diskfs directly" is enforced only by review.

## Locked decisions

- **Targeted rules only.** `linters.default: standard` is *exactly* the
  no-config set — introducing this file must not change which findings CI
  reports, only add depguard's. No revive/misspell/godot/errorlint; that was
  considered and declined.
- depguard scopes a **whole rule** to a file set via `rules.<name>.files`
  (globs, `!` negates, plus the `$test`/`$all` expanders). It has no
  per-package allow list, so "restrict go-diskfs to three packages" is written
  as one deny rule with an all-negation `files` list. Prefix matching on
  `deny[].pkg` covers all six go-diskfs import paths in use with one entry.
- **Exempt `internal/diskfmt`, `internal/image`, `internal/qemurun`, and
  `$test`.** CLAUDE.md's sentence is narrower than reality: three non-test
  packages import go-diskfs legitimately — `internal/image` is the image
  assembler (its own doc comment says so) and `internal/qemurun` opens an image
  read-only. Eight test files open a built `.img` to assert on its contents,
  which is the verification idiom CLAUDE.md itself mandates; depguard cannot
  tell a read from a write.
- **Do not port a v1-shaped config.** v1's `issues.exclude-dirs-use-default`
  has no v2 equivalent; a ported file would silently stop linting `examples/`.
- Starting point (refine as needed, but keep the comments — the `desc` is what
  a developer actually reads):

```yaml
version: "2"
linters:
  default: standard
  enable: [depguard]
  settings:
    depguard:
      rules:
        go-diskfs:
          list-mode: lax
          files: ["!**/internal/diskfmt/*.go", "!**/internal/image/*.go", "!**/internal/qemurun/*.go", "!$test"]
          deny:
            - pkg: github.com/diskfs/go-diskfs
              desc: >-
                Use internal/diskfmt's wrappers for FAT32 work: go-diskfs
                under-sizes FATs, trims label spaces per 8.3 field, and hides
                leading-dot filenames from its own listings.
```

## Todo

- [x] Write `.golangci.yml`
- [x] `golangci-lint config verify` — clean (and see the last todo: `run` does NOT verify)
- [x] Confirm the finding set is unchanged apart from depguard (compare a run with and without the file)
- [x] Verify it bites: add a go-diskfs import to a non-exempt package
- [x] Amend CLAUDE.md's go-diskfs bullet: name all three owning packages, and say a violation now surfaces as a lint finding rather than a review comment
- [x] Consider adding `golangci-lint config verify` as a step in ci.yml's `lint` job — not needed, the pinned action already does it
- [x] Quality gates — `GOOS=linux golangci-lint run ./...` is the load-bearing one here (`internal/qemurun` and the `_linux.go` split)

## Notes

No changeset — internal only, use the `no release notes` label.

## Summary of Changes

Added `.golangci.yml` — the repo's first — carrying one depguard rule and
nothing else, and corrected CLAUDE.md's go-diskfs bullet to match reality.

**Config.** `version: "2"`, `linters.default: standard`,
`linters.enable: [depguard]`, and the bean's starting-point rule kept
verbatim: `list-mode: lax`, an all-negation `files` list
(`!**/internal/diskfmt/*.go`, `!**/internal/image/*.go`,
`!**/internal/qemurun/*.go`, `!$test`) and a single prefix `deny` entry for
`github.com/diskfs/go-diskfs`. Comments at the top state why the file exists
and that adding linters is a separate, deliberate decision.

**Evidence the finding set is otherwise unchanged.** `golangci-lint linters`
diffed before/after is a two-line diff — depguard moving from the "Disabled
by your configuration" list to the "Enabled" one; the standard five
(errcheck, govet, ineffassign, staticcheck, unused) are untouched.
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...` report
`0 issues` both with and without the file. `examples/` is still linted —
proven positively, not by inspection: a probe import placed in
`examples/hello` was reported.

**Evidence the rule bites.** Temporary probe files importing
`github.com/diskfs/go-diskfs`, `.../partition/mbr` and
`.../filesystem/fat12` from `examples/hello` and `internal/pipeline` all
produced findings under both GOOS, so the single `deny` prefix does cover
every subpackage in use. The three owning packages and all 8 go-diskfs-
importing test files stayed silent throughout. That silence was shown to be
the exemption rather than the files being out of scope: deleting `!$test`
from the list surfaces all 8 test files (11 findings) immediately.

**Two findings that contradict the bean's framing.**

1. The bean expected a schema typo to fail the lint job. It does not:
   `golangci-lint run` (v2.12.2) silently ignores an unknown key —
   `list-modes:` for `list-mode:` linted happily, with the option quietly
   defaulting. The real hazard is therefore a silently *widened* fence, not
   a red job.
2. Which makes verification worth having — but the last todo still resolves
   as "no ci.yml change". `golangci/golangci-lint-action` v9 has a `verify`
   input defaulting to `true` and runs `<bin> config verify` itself whenever
   a config file is present (confirmed in the pinned bundle's `runVerify`).
   Before this PR there was no config for it to check; from now on CI
   verifies the schema on every run for free. `golangci-lint config verify`
   also passes locally.

**Scope note.** `**/internal/diskfmt/*.go` matches that directory only, so
the `internal/diskfmt/ext4golden` subpackage is fenced like everything else.
It imports no go-diskfs today and has no reason to; the tighter glob is
deliberate.

**CLAUDE.md.** The "FAT32 work goes through `internal/diskfmt`'s wrappers"
bullet now names all three legitimate importers (`internal/diskfmt`,
`internal/image`, `internal/qemurun`), says tests may import it too, and
records that the fence is a depguard rule rather than a review obligation —
plus a line pinning the file's purpose, so a later reader doesn't mistake it
for an invitation to enable more linters.
