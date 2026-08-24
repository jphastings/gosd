---
gosd: minor
---

#### App versions can resolve from git tags: `--app-version git:v*.*.*`

A version recorded in a checked-in `gosd-build.toml` couldn't change per
release — so now it doesn't have to. An `[app] version` (or
`--app-version`) value starting `git:` resolves at build time from your
app repository's tags, in pure Go with no git binary required: the
wildcard pattern after `git:` picks which tags count, the matching tag
nearest the commit being built wins (describe semantics, so maintenance
branches never steal a newer tag from another branch), and the pattern's
literal prefix is stripped from the result — `git:v*.*.*` turns tag
`v1.4.2` into `1.4.2`. Between releases the version reads
`1.4.2-5-g<hash>`, and an unclean worktree appends `-dirty` rather than
failing the build.

Shallow CI checkouts have no tags to search, so the error for that case
names the fix directly: `fetch-depth: 0` for actions/checkout, or
`git fetch --unshallow --tags`. Details and examples live on the
build-config documentation page.

One edge: a literal version that genuinely began with `git:` now means
something else; no known app does this.
