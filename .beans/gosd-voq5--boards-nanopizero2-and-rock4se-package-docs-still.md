---
# gosd-voq5
title: 'boards: nanopizero2 and rock4se package docs still claim internal-only registration; build.go comment cites wrong bean'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:11Z
updated_at: 2026-07-31T07:54:11Z
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
