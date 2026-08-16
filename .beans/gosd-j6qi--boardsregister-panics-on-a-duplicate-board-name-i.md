---
# gosd-j6qi
title: 'boards.Register panics on a duplicate board name instead of failing with an actionable message'
status: todo
type: task
priority: low
created_at: 2026-08-16T04:43:32Z
updated_at: 2026-08-16T04:43:32Z
---

**Severity: Low.** Narrow blast radius — this only fires if a new board
package is added with a copy-pasted, un-edited name constant — but when it
does fire, it's a bare panic stack trace at process init rather than the
actionable CLI error this project holds every other user-facing failure to.

## Verified

`internal/boards/boards.go:280-289`:

```go
func register(b Board, internal bool) {
	name := b.Name()
	if _, exists := boards[name]; exists {
		panic(fmt.Sprintf("boards: %q is already registered", name))
	}
	...
}
```

Called from `Register`/`RegisterInternal`, which are invoked as explicit,
ordered calls from `cmd/gosd/build.go`'s init-time registration block — not
from each board package's own `init()`. The most plausible way to trip this
is copy-pasting an existing board package to scaffold a new one (a very
likely path for board #8+) and forgetting to change the `boardName`
constant: the CLI would panic at startup with a raw stack trace, not with
the "X failed because Y; try Z" shape CLAUDE.md's code-conventions section
requires of every other CLI-facing error.

## Fix

This is a programmer error, not a runtime condition — a `panic` at package
init is a defensible choice in general — but the message can still name the
fix. Something like:

```go
panic(fmt.Sprintf("boards: %q is already registered — if you're adding a new board, "+
	"check its Name() wasn't copy-pasted from the board you scaffolded it from", name))
```

is a one-line change that turns "confusing stack trace" into "obvious fix,"
with no behavior change otherwise.

## Todos

- [ ] Reword the panic message in `boards.go:283`
- [ ] Consider whether `internal/boards`' own tests already cover the
      duplicate-registration path (a quick grep suggests they don't) and add
      one if not — asserting the panic fires and its message names the cause
