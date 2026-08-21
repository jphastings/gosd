---
# gosd-zj13
title: 'gosd build --env-file: app-supplied [env] docs and commented-out suggestions'
status: scrapped
type: feature
priority: normal
created_at: 2026-08-09T02:46:52Z
updated_at: 2026-08-20T05:36:42Z
---

## Reasons for Scrapping

**Obsolete: the flag this bean extends no longer exists.** It specifies
`gosd build --env-file <path>`, a verbatim TOML splice into `gosd.toml`'s
`[env]` section, implemented as `gosdtoml.ParseEnvBody` and
`gosdtoml.EnvSection{Verbatim}`. None of that is in the tree —
`internal/gosdtoml` is gone, `--env-file` is gone, and so are `--env`,
`--hostname`, `--wifi-ssid` and `--wifi-pass`. `grep -r "env-file"
--include='*.go' .` finds one unrelated test name.

The per-attribute config tree replaced the whole hand-editable file (epic
`gosd-rw6n`, decided 2026-08-12/13; see the locked decision in the root
CLAUDE.md and the configuration docs).

**What supersedes it, feature for feature.** This bean wanted two things
that gosd.toml's rendered `[env]` section couldn't give an app developer:

- *Per-key explanatory comments.* The tree requires them. Every value file
  needs a `<name>.explain.md` sidecar — its own or inherited — and the build
  **refuses** an undocumented setting outright (`configtree.checkValue`),
  which is stronger than the optional comments this bean proposed. The
  developer's authoring surface is `gosd build --config-dir`, a directory
  overlaid file-by-file onto gosd's own defaults.
- *Suggested-but-inactive keys the user opts into.* An empty value file is
  how the tree says "not set", and it ships padded to its reservation, so a
  documented `config/env/RUN_DEMO` with an empty value is exactly the
  commented-out suggestion this bean asked for — except it is a real file
  with real documentation beside it, and a person fills it in rather than
  uncommenting TOML syntax.

Nothing is left to carry forward: the requirement is met and the
implementation it specified cannot be built. Not resurrecting `--env-file`.

Re-validated and scrapped 2026-08-20, alongside `gosd-m09v`, `gosd-q4v5` and
`gosd-15ld` — three sibling security beans filed against the same removed
file, of which those three described defects that outlived it and this one
did not.
