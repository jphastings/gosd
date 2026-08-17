---
# gosd-qs2g
title: Validate change files in-tree, so a quoted knope key can't fail silently
status: todo
type: task
priority: normal
created_at: 2026-08-17T16:56:30Z
updated_at: 2026-08-17T16:57:48Z
parent: gosd-8pgg
blocked_by:
    - gosd-ihdn
---

Part of epic gosd-8pgg. Stacked on gosd-ihdn (needs `internal/repocheck`).
Independent of the other repocheck children — different file, no conflict.

`change-file-check.yml` asks "does this PR diff add a `.changeset/*.md`?" and
nothing more. Nothing checks the file is *valid*, and the failure mode is
silent: docs/releasing.md warns that a **quoted** frontmatter key
(`"npm/gosd": patch`) parses without error and contributes nothing to any
package's release. You find out at release time, or never.

## Locked decisions

- A Go test in `internal/repocheck/changeset_test.go`, not a CI shell step:
  `go test ./...` is a mandated local gate, so an agent catches it before
  pushing.
- **This complements `change-file-check.yml`, it does not replace it.** That
  workflow's question is inherently base-ref-scoped and stays a workflow; this
  one asks "is every change file in the tree valid?". Say so in the test doc.
- **Do not parse the frontmatter as YAML.** `yaml.Unmarshal` maps
  `"npm/gosd": patch` and `npm/gosd: patch` to the *same* Go map key, so a YAML
  round-trip is structurally blind to exactly the quoting knope ignores. The
  check must be lexical on the raw lines.
- Each frontmatter line must match `^([A-Za-z0-9/_.-]+): +(major|minor|patch|note)$`
  — the key group **deliberately excludes quote characters**, so a quoted key
  fails with a message naming knope's silent-ignore behaviour.
- **Derive the valid package keys from `knope.toml`**, don't hardcode them:
  `^\[packages\.(?:"([^"]+)"|([A-Za-z0-9_-]+))\]` yields `gosd`, `artifacts`,
  `npm/gosd`. No TOML dependency — the repo has none and this doesn't justify
  one. Note the trap this catches: knope.toml *must* quote `npm/gosd` (TOML
  requires it) while the change file must *not*.
- `note` is valid for `gosd` only (knope.toml's `extra_changelog_sections`
  under `[packages.gosd]`). Hardcoding that one fact with a pointer at the
  knope.toml line is fine — deriving it is worse than the duplication.
- Also require: at least one package line, and a non-empty body whose first
  non-blank line starts `#### ` (docs/releasing.md prescribes it; a missing
  heading renders wrong in CHANGELOG.md).

## Todo

- [ ] `internal/repocheck/changeset_test.go`
- [ ] Verify it bites: quote a key in a real `.changeset/*.md` and confirm the message names the silent-ignore
- [ ] Amend CLAUDE.md's change-file bullet — "package keys unquoted" is now enforced, so point at the test instead of restating the rule
- [ ] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

No changeset — internal only, use the `no release notes` label. (Worth saying
in the PR body, given this PR adds the very check that would police one.)
