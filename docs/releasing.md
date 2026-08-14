# Releasing: change files, the release PR, and what merging it does

GoSD ships three independent release surfaces — the `gosd` CLI, board
artifacts (kernels/U-Boot), and the `@jphastings/gosd` npm package — and
releases every one of them through [knope](https://knope.tech/), configured
in `knope.toml`. This repo does not use conventional commits: a small
markdown **change file** is the only input to what gets released and what
its notes say.

## The flow

1. A PR with a user-facing change adds a change file under `.changeset/`.
2. A bot-maintained pull request titled "chore: prepare release" (branch
   `knope/release`) accumulates every pending change file. It's refreshed —
   version bumps and changelog entries recomputed — on every push to `main`
   that has at least one pending change file.
3. Merging that PR is the deliberate release act. knope tags each package
   that has pending changes and creates a GitHub release for it, with notes
   drawn straight from the change files. The artifacts and npm pipelines
   fire from their tags exactly as they did before knope — nothing about
   `build-artifacts.yml` or `publish-npm.yml`'s triggers changed.

Until a change file lands, there's nothing to release: the release PR
simply doesn't exist. It can also sit open indefinitely — see "combined-PR
ordering discipline" below.

## Change-file format

A change file is markdown with YAML frontmatter mapping package name to
bump type:

```markdown
---
gosd: minor
---

#### Short title for the release notes

Prose explaining the change, written for someone reading the release, not
someone reading the diff.
```

The package names are `gosd`, `artifacts`, and `npm/gosd`. A single change
file may list more than one package if a PR affects more than one surface.
Bump types are `major`, `minor`, or `patch`; the `gosd` package also accepts
`note`, which lands its entry in a "Notes" changelog section — useful for a
call-out that isn't a feature or fix. A `note` still implies a patch bump:
there is no bump-free change type, so use it only for call-outs that
deserve a release of their own.

> **Warning:** frontmatter keys must be **unquoted**. `npm/gosd: patch`
> works; `"npm/gosd": patch` is silently ignored by knope — the change file
> parses without error but contributes nothing to any package's release.

The heading and prose below the frontmatter become the release note itself,
so write it as one: what changed and why it matters, not an internal diff
summary.

Humans can generate a change file interactively with `knope document-change`
(the `document-change` workflow in `knope.toml`); agents write
`.changeset/*.md` by hand in the same format.

## 0.x versioning rules

While the major version stays at `0`, knope maps bump types down a rung: a
`major` change file bumps the minor component, and `minor`/`patch` change
files both bump the patch component. In practice this means ordinary new
features produce patch releases until the CLI reaches `1.0`. To cut a
specific version number instead — for a deliberate `1.0.0`, say — run
`knope prepare-release --override-version <package>=<version>` locally
rather than relying on the workflow.

## The `no release notes` label

Not every PR has a release note: refactors, CI changes, beans-only commits.
Apply the `no release notes` label instead of adding a change file — the
change-file check (`.github/workflows/change-file-check.yml`) accepts
either.

## What merging the release PR does, per surface

- **gosd** — a `gosd/vX.Y.Z` GitHub release, plus the plain `vX.Y.Z`
  Go-module tag that `go install github.com/jphastings/gosd/cmd/gosd@latest`
  resolves against.
- **artifacts** — an `artifacts/vX.Y.Z` GitHub release. The tag alone
  doesn't carry compiled kernels: those assets are attached 20–60 minutes
  later by the same [artifact build pipeline](artifacts.md) that ran before
  knope existed. The `internal/artifacts.Version` pin bump that makes newly
  built `gosd` binaries consume the new release stays a separate follow-up
  PR, tag-first/bump-second, exactly as described there.
- **npm/gosd** — an `npm/gosd/vX.Y.Z` GitHub release, which triggers the
  existing staged, tokenless publish to the npm `next` dist-tag. Promoting
  a version to `latest` is still a manual step — see
  [publishing js/packages/\*](../js/PUBLISHING.md).

## Combined-PR ordering discipline

Merging the release PR releases **every** package that currently has
pending change files — there's no way to merge it partially. To release
only one surface, merge it while only that surface has pending changes
(hold the others by not merging, or by not yet having added their change
files). This keeps the artifacts cadence unchanged: an artifacts release,
its assets landing later, then the separate pin-bump PR, then whatever CLI
release picks it up next — knope doesn't collapse that sequence, it only
automates the tagging step at each stage.

## Manual escape hatch

Hand-pushing an `artifacts/vX.Y.Z` tag still works for emergencies, but
create its GitHub release first (`gh release create artifacts/vX.Y.Z`) —
the artifact build workflow only uploads assets to an *existing* release,
it no longer creates one from a bare tag push. The npm tag needs no such
step, and a hand-pushed plain `vX.Y.Z` tag has no automation attached at
all.
