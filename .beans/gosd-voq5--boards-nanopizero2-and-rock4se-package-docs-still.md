---
# gosd-voq5
title: 'boards: nanopizero2 and rock4se package docs still claim internal-only registration; build.go comment cites wrong bean'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:11Z
updated_at: 2026-08-07T18:13:19Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

nanopizero2/board.go:9-19 still says "INTERNAL-ONLY FOR NOW... no
artifacts-pipeline job... a real build would 404"; rock4se/board.go:11-13
says "Registered internal ... until bean gosd-0vvh flips it public". Both
boards are publicly registered in cmd/gosd/build.go (activations:
gosd-wskc / artifacts v0.2.0, gosd-h8a8 / v0.5.0), in the default build
set and catalog. rock4se's comment also names the wrong bean (gosd-0vvh
vs the actual activation gosd-h8a8).

**Fix:** rewrite both package headers to match radxazero3e's (which has no
stale wording); correct the bean reference.



## Summary of Changes

Verified both claims against current code before editing:

- `cmd/gosd/build.go` registers both boards publicly (`boards.Register`),
  confirming both are public — the internal-only wording in their package
  docs was stale.
- `internal/boards/nanopizero2/board.go`: deleted the stale "INTERNAL-ONLY
  FOR NOW" paragraph (registration flipped in commit c1fbbef, bean
  gosd-wskc); the remaining doc now matches radxazero3e's shape (boot-chain
  description, then build/artifact-resolution + locked-offsets references).
- `internal/boards/rock4se/board.go`: replaced the stale "Registered
  internal ... until bean gosd-0vvh flips it public" sentence with
  "Activated as a public board once its artifact release landed (bean
  gosd-h8a8)" — gosd-0vvh was the original board-profile bean;
  gosd-h8a8 is the actual activation bean (public registration + v0.5.0
  artifacts, commit 2efded9).
- No behavior changes; comments/docs only. Confirmed `build.go`'s own
  registration comments (nanopi-zero2: gosd-f39b, rock-4se: gosd-h8a8) were
  already correct, so no edit was needed there.

Gates run: `go test ./...`, `go vet ./...`, `gofmt -l .`,
`golangci-lint run --allow-parallel-runners ./...`,
`GOOS=linux golangci-lint run --allow-parallel-runners ./...` — all clean.
