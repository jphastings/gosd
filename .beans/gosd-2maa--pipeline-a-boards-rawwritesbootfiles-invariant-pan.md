---
# gosd-2maa
title: 'pipeline: a boards RawWrites/BootFiles invariant panic reaches the user as a raw stack trace'
status: completed
type: task
priority: normal
created_at: 2026-07-31T07:54:11Z
updated_at: 2026-08-20T06:26:47Z
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

## Summary of Changes

`internal/pipeline/pipeline.go`: added `callBootFiles`/`callRawWrites`, thin wrappers around `Board.BootFiles`/`Board.RawWrites` that `recover()` an invariant-violation panic and turn it into a normal `error`, preserving the panic's own message verbatim. Board packages deliberately panic for conditions that can only be programmer/build-config mistakes (an artifact the board's own `Artifacts()` didn't declare, e.g. `Artifacts.MustOpen`; an output too big for a board's locked raw-write layout, e.g. radxazero3e's u-boot.itb size check) and already write actionable messages when they do — the bug was purely that nothing between those panics and `main()` recovered them, so they reached the user as a raw Go stack trace instead of the single-line CLI error every other build failure produces. `Assemble`'s two call sites now go through these wrappers and wrap the resulting error with the same "assembling boot files for %s"/"computing raw writes for %s" context the existing non-panic error paths already used, so `errors.Is`/message-content behavior for a normal returned error is unchanged.

Added `internal/pipeline/pipeline_test.go` tests `TestAssembleTurnsABoardBootFilesPanicIntoAnActionableError` and `TestAssembleTurnsABoardRawWritesPanicIntoAnActionableError`, each driving a fake board that panics and asserting `Assemble` returns a normal error containing the panic's message, rather than crashing the test process.
