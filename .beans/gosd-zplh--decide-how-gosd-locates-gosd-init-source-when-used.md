---
# gosd-zplh
title: Decide how gosd locates gosd-init source when used outside its own repo
status: completed
type: task
priority: normal
created_at: 2026-07-03T08:41:14Z
updated_at: 2026-07-05T06:10:23Z
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

## Summary of Changes

Implemented option 2 (build from the module cache) as a three-rung ladder in
`internal/build/gosdinit.go`, replacing the old "always build via
`github.com/jphastings/gosd/cmd/gosd-init` import path" approach that only
worked inside gosd's own checkout:

1. **Dev checkout (unchanged workflow):** if the module rooted at the current
   working directory is `github.com/jphastings/gosd`, or this gosd binary was
   itself compiled from a checkout still present on disk (detected via this
   file's own compile-time source path, `runtime.Caller`), gosd-init is built
   straight from that checkout via `go -C <dir> build ./cmd/gosd-init`.
2. **Module cache:** otherwise, `runtime/debug.ReadBuildInfo` gives gosd's own
   build version; `go mod download -json github.com/jphastings/gosd@<version>`
   fetches (or reuses) that exact version in the local module cache, and
   gosd-init is built from the resulting directory the same way (`go -C <dir>
   build`, never writing into the — possibly read-only — module cache). A
   `(devel)` version (no real release) is rejected with an actionable error
   pointing at building from a checkout or `--gosd-init-src`.
3. **Escape hatch:** `--gosd-init-src <dir>` (documented in `--help`, not in
   the README quickstart) always wins and skips detection.

### Verification (acceptance: `go install .../gosd@latest` in a scratch
module, then `gosd build .` reaches image assembly with a compiled
gosd-init)

No real tagged release exists yet, so `@latest` itself can't be exercised.
What I did verify, end-to-end, in a scratch module outside this repo:

- **Rung 1 (dev checkout), both variants:** `go install ./cmd/gosd` from this
  branch's worktree (version `(devel)`), then running that binary's `gosd
  build .` from an unrelated scratch app module — resolution fell through to
  the compiled-in source path fallback (working directory's module isn't
  gosd) and built gosd-init from the worktree checkout, reaching image
  assembly. Also re-checked the plain "cwd is gosd's own module" path via
  `go run ./cmd/gosd build ./examples/hello` from inside the checkout.
- **Rung 3 (`--gosd-init-src`):** ran the scratch-installed binary with
  `--gosd-init-src <worktree>/cmd/gosd-init`, reached image assembly.
- **Rung 2 (module cache):** simulated a real tagged release without pushing
  one, by serving this branch's tree as `github.com/jphastings/gosd@v0.1.0`
  from a hand-built local file-based Go module proxy (`GOPROXY=file://...`),
  then `go install github.com/jphastings/gosd/cmd/gosd@v0.1.0` from that
  proxy (third-party deps came from the real proxy.golang.org). The resulting
  binary correctly embeds version `v0.1.0` (`go version -m` confirms no
  `-trimpath`, so I then deleted the extracted module-cache directory *and*
  its download cache entry for that version to rule out rung 1's compiled-path
  fallback silently succeeding on the same directory) — `gosd build .` from
  the scratch app then re-populated the module cache via `go mod download`
  and reached image assembly, confirming rung 2's `go mod download` path
  works for real.
- **Not exercised:** the true `@latest`/real-tag path, and the `(devel)`
  error path in a scenario where rung 1 can't find a checkout at all (that
  error message is covered by a unit test instead, since `go test` binaries
  always report their own module as `(devel)`).

Also added unit tests in `internal/build/gosdinit_test.go` covering the
default ladder, the override, missing-override-dir errors, and the `(devel)`
rejection; recorded the decision as a one-line entry in CLAUDE.md's locked
decisions.
