# Per-board build tags: gating app source per board

`gosd build` compiles your app once per selected board, passing a real Go
build tag identifying that board. Keep board-specific source (different pin
numbers, an optional peripheral, a board-only feature) in your own app and
the right file is selected automatically — no gosd-specific SDK or import
required.

## The tag

For board id `<id>`, the tag is `gosd_<id>` with hyphens replaced by
underscores:

| Board ID | Build tag |
|---|---|
| `pi-zero-2w` | `gosd_pi_zero_2w` |
| `pi-zero-w` | `gosd_pi_zero_w` |
| `pi-3b` | `gosd_pi_3b` |
| `radxa-zero-3e` | `gosd_radxa_zero_3e` |
| `nanopi-zero2` | `gosd_nanopi_zero2` |
| `rock-4se` | `gosd_rock_4se` |

Gate a file to a board with a `//go:build` constraint:

```go
//go:build gosd_pi_zero_2w

package main

// pi-zero-2w-specific code here.
```

`gosd build` passes this tag to the app compile only — never to gosd-init,
and never as a filename convention gosd itself interprets (see below).

## The fallback pattern

`gosd build` is the only thing that ever passes a `gosd_*` tag, so plain `go
build ./...` and `go test ./...` — CI, your editor, anyone building your app
without gosd — see none of them set. A file gated to one board is invisible
in that build, so any symbol it defines needs a fallback, or a plain build
fails outright.

Two ways to provide one:

1. **A default file with a negated constraint**, covering the case where no
   board-specific tag is set:

   ```go
   //go:build !gosd_pi_zero_2w && !gosd_nanopi_zero2

   package main

   // Default/fallback implementation.
   ```

2. **The board-gated files are the sole definers of the symbol**, and
   something else (a different, always-compiled file) only ever calls it
   through an interface or function variable set from an `init()` in each
   variant — so there's nothing left for a plain build to fail to resolve.

Either way, the goal is the same: a plain `go build ./...` must stay clean
with no board tag set.

## The `_<board>.go` filename suffix is cosmetic only

Naming a file `stuff_pi-zero-2w.go` does **not** gate it to that board. Go's
filename-based build constraints only recognize known `GOOS`/`GOARCH`
suffixes (`_linux.go`, `_arm64.go`, ...), and a board id like `pi-zero-2w` is
neither — so a file named that way compiles into **every** build. Use a
`_<board>.go`-style suffix for readability if you like, but it's just a
naming convention: the `//go:build gosd_<id>` line is what actually gates
the file, and it must always be present explicitly.
