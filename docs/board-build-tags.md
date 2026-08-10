# Build tags: gating app source on gosd, and per board

`gosd build` compiles your app once per selected board, passing two real Go
build tags: `gosd`, the same for every build gosd performs, and one
identifying the board. Keep source that only makes sense on a device (or only
on one board — different pin numbers, an optional peripheral) in your own app
and the right file is selected automatically — no gosd-specific SDK or import
required.

## The `gosd` tag

Every app compile `gosd build` and `gosd run` perform sets the bare `gosd`
tag. It means "this source is being compiled into an image", so an app can
behave differently on a device without maintaining a separate main package:

```go
//go:build gosd

package main

// Real hardware: /data is a mounted partition, GPIO exists, exit means reboot.
```

```go
//go:build !gosd

package main

// Desktop/CI: a temp dir, fake peripherals, exit means exit.
```

Nothing but gosd ever sets it, so plain `go build ./...`, `go test ./...`,
your editor and CI all see the `!gosd` side.

## The per-board tag

For board id `<id>`, the second tag is `gosd_<id>` with hyphens replaced by
underscores:

| Board ID | Build tag |
|---|---|
| `pi-zero-2w` | `gosd_pi_zero_2w` |
| `pi-zero-w` | `gosd_pi_zero_w` |
| `pi-3b` | `gosd_pi_3b` |
| `radxa-zero-3e` | `gosd_radxa_zero_3e` |
| `nanopi-zero2` | `gosd_nanopi_zero2` |
| `rock-4se` | `gosd_rock_4se` |
| `cubie-a5e` | `gosd_cubie_a5e` |

Gate a file to a board with a `//go:build` constraint:

```go
//go:build gosd_pi_zero_2w

package main

// pi-zero-2w-specific code here.
```

Both tags go to the app compile only — never to gosd-init, and neither is a
filename convention gosd itself interprets (see below).

## The fallback pattern

A file gated to one board is invisible in a plain build, so any symbol it
defines needs a fallback, or `go build ./...` fails outright.

Three ways to provide one:

1. **Negate the `gosd` tag.** One constraint covers every board at once, and
   keeps working as boards are added:

   ```go
   //go:build !gosd

   package main

   // Default/fallback implementation.
   ```

2. **A default file with negated board constraints**, when the fallback is
   only for *some* boards and other boards should still share it:

   ```go
   //go:build !gosd_pi_zero_2w && !gosd_nanopi_zero2

   package main
   ```

   This one needs editing whenever you add a board to the set — prefer
   `!gosd` where it does the job.

3. **The gated files are the sole definers of the symbol**, and something
   else (a different, always-compiled file) only ever calls it through an
   interface or function variable set from an `init()` in each variant — so
   there's nothing left for a plain build to fail to resolve.

Either way, the goal is the same: a plain `go build ./...` must stay clean
with no gosd tag set.

## The `_<board>.go` filename suffix is cosmetic only

Naming a file `stuff_pi-zero-2w.go` does **not** gate it to that board. Go's
filename-based build constraints only recognize known `GOOS`/`GOARCH`
suffixes (`_linux.go`, `_arm64.go`, ...), and a board id like `pi-zero-2w` is
neither — so a file named that way compiles into **every** build. Use a
`_<board>.go`-style suffix for readability if you like, but it's just a
naming convention: the `//go:build` line is what actually gates the file, and
it must always be present explicitly.
