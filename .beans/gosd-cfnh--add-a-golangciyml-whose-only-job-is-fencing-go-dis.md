---
# gosd-cfnh
title: Add a .golangci.yml whose only job is fencing go-diskfs to its three owners
status: todo
type: task
created_at: 2026-08-17T16:56:12Z
updated_at: 2026-08-17T16:56:12Z
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

- [ ] Write `.golangci.yml`
- [ ] `golangci-lint config verify` — a schema typo fails the whole lint job and reads like a code problem
- [ ] Confirm the finding set is unchanged apart from depguard (compare a run with and without the file)
- [ ] Verify it bites: add a go-diskfs import to a non-exempt package
- [ ] Amend CLAUDE.md's go-diskfs bullet: name all three owning packages, and say a violation now surfaces as a lint finding rather than a review comment
- [ ] Consider adding `golangci-lint config verify` as a step in ci.yml's `lint` job
- [ ] Quality gates — `GOOS=linux golangci-lint run ./...` is the load-bearing one here (`internal/qemurun` and the `_linux.go` split)

## Notes

No changeset — internal only, use the `no release notes` label.
