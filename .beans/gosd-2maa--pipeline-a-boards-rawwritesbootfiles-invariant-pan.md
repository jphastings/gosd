---
# gosd-2maa
title: 'pipeline: a boards RawWrites/BootFiles invariant panic reaches the user as a raw stack trace'
status: todo
type: task
priority: normal
created_at: 2026-07-31T07:54:11Z
updated_at: 2026-07-31T07:54:11Z
---

Found by review sweep `gosd-fuxs` (build pipeline area), verified.

Board packages deliberately panic on invariant violations (e.g. u-boot.itb
too big, radxazero3e/board.go:139-146) with excellent messages — but
nothing between `Board.RawWrites`/`BootFiles` and main() recovers
(internal/pipeline/pipeline.go:183-189), so the user sees a Go stack
trace instead of the single-line actionable error the project requires.
Becomes more reachable once gosd-fija adds the idbloader overlap check.

**Fix:** recover() around the pipeline's board calls (or cmd/gosd's
Execute), reformatting a boards-package panic into a normal wrapped CLI
error carrying the panic message.
