---
# gosd-zplh
title: Decide how gosd locates gosd-init source when used outside its own repo
status: todo
type: task
created_at: 2026-07-03T08:41:14Z
updated_at: 2026-07-03T08:41:14Z
parent: gosd-vi0n
---

Surfaced by the scaffold implementation: `gosd build` cross-compiles gosd-init via its module import path, which only works when gosd runs from inside its own module checkout. A developer who `go install`s gosd and runs it in THEIR app repo has no gosd-init source on disk.

Options to evaluate (pick one, document in CLAUDE.md):
1. Embed prebuilt gosd-init arm64 binaries into the gosd binary via go:embed at release time (simple for users; fattens the CLI; needs a release build step)
2. Build gosd-init from the module cache: resolve github.com/jphastings/gosd@<own version> via `go mod download` and compile from there (keeps everything source-built; needs network/module cache and version pinning care)
3. Require gosd as a go.mod tool dependency and build via the user module context

Recommendation to start from: option 2, falling back to 1 if the module-cache dance proves fragile. Whatever is chosen must keep `gosd build` working offline after first use (CLAUDE.md artifact-cache promise).

## Acceptance
`go install github.com/jphastings/gosd/cmd/gosd@latest` in a scratch module unrelated to this repo, then `gosd build .` reaches image assembly with a compiled gosd-init.
