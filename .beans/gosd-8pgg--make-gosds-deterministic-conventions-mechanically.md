---
# gosd-8pgg
title: Make GoSD's deterministic conventions mechanically enforced
status: in-progress
type: epic
priority: normal
created_at: 2026-08-17T16:55:14Z
updated_at: 2026-08-17T16:57:57Z
---

CLAUDE.md carries ~40 locked decisions. Some are already enforced by code; several
are enforced only by prose plus JP's review, and those are the ones that have
actually drifted — a stale board list here, a missing CI target there. This epic
moves every *deterministic* rule into a mechanism that fires before review,
preferably one an agent already runs.

## Locked decisions

- **Three layers, no more.** Repo-invariant `go test` cases (the primary
  mechanism), a `.golangci.yml` with targeted rules only, and Claude Code
  hooks for agent-workflow rules. **No new CI/PR-metadata checks** — no branch
  naming gate, no bean-file-in-PR gate; `change-file-check.yml` stays the only
  PR-metadata workflow.
- **`go test` is the preferred home**, not shell in CI. `go test ./...` is a
  mandated local gate, so an agent catches the problem before pushing.
  `internal/kernelspec/workflow_test.go` is the precedent: it YAML-parses a
  workflow file and asserts parity in both directions, deriving names from a
  convention rather than a hard-coded list. Mirror its shape.
- **golangci-lint: targeted rules only.** No new quality linters (no revive,
  misspell, godot…). `linters.default: standard` must reproduce today's
  no-config behaviour exactly; the file exists only to add depguard.
- **The doc-link rule lands as a ratchet**, not a remediation. ~103 of 141 doc
  references violate it; per-file counts that may not rise.
- **Every check derives its expectations**; a hand-maintained list inside a
  check recreates the drift the check exists to catch.
- **Each PR amends the CLAUDE.md prose its mechanism replaces**, in the same PR.
  A rule that is now enforced should shrink to a pointer, not sit there twice.

## Explicitly out of scope

Already enforced, do not rebuild: `.explain.md` sidecars and 256-byte config
padding (build-time refusals in `internal/configtree`), `CGO_ENABLED=0` and the
non-Linux stubs (CI cross-compiles + the macos-latest test leg), per-board CI
kernel jobs (`internal/kernelspec/workflow_test.go`).

Rejected as unlintable: the `platform_linux.go`/`platform_other.go` shape (six
legitimate exceptions, and "compiles on darwin" is already CI-enforced); commit
message style (last 60 commits ~100% compliant; a conventional-commit preset
would regress the house style).

## Children

Each child is one branch, one PR. The first is a structural prerequisite the
rest stack on; everything else is mutually independent.


## Children (created 2026-08-17)

| Bean | Work | Branches from |
|---|---|---|
| gosd-ihdn | `internal/boardset` + `internal/repocheck` scaffolding | main |
| gosd-x915 | Board parity derived from the registry | gosd-ihdn |
| gosd-cfnh | `.golangci.yml` + depguard fence for go-diskfs | main |
| gosd-qs2g | Change-file validity | gosd-ihdn |
| gosd-nw6e | Bean-reference integrity + doc-link ratchet | gosd-ihdn |
| gosd-asdg | examples cross-compile coverage + `wiphyDumpFlags` | gosd-ihdn |
| gosd-bn6j | Claude Code hooks | main |

gosd-ihdn is the only blocker; the four beans stacked on it touch different
files and are mutually independent. Retarget the survivors onto main once
gosd-ihdn merges, and verify the content actually reached main rather than
trusting a badge.
