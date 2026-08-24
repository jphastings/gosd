---
gosd: minor
---

#### `gosd build` accepts `go build`'s own compile flags

`gosd build` (and `gosd-build.toml`) now accepts five flags that mirror `go
build` itself, applied to your app's own compile only (never gosd-init):

- `--ldflags` — e.g. `--ldflags="-X main.version=1.4.2"` to stamp a version
  into the compiled binary, closing a real gap: until now nothing gosd
  built ever put a version into the app binary itself (`--app-version`
  bakes into `config.json`/crash reports only).
- `--tags` — extra Go build tags for your app compile, comma- or
  space-separated. These **merge** with gosd's own mandatory
  `gosd`/`gosd_<board>` tags rather than replacing them; a `gosd` or
  `gosd_`-prefixed value is refused, since gosd always adds those itself.
- `--trimpath`, `--gcflags`, `--asmflags` — passed straight through to `go
  build`.

None of the five are mirrored on `gosd run`, matching its existing pattern
of not exposing every build flag. See the build-config docs and
`docs/board-build-tags.md` for details.
