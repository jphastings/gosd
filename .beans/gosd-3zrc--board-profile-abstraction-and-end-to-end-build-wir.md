---
# gosd-3zrc
title: Board profile abstraction and end-to-end build wiring
status: completed
type: task
priority: normal
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-03T23:21:07Z
parent: gosd-vi0n
blocked_by:
    - gosd-vq4g
    - gosd-cvzt
---

Define `internal/boards.Board` and wire `gosd build` end to end: compile app + gosd-init → build initramfs → assemble boot files → write image.

Interface (locked):
```go
type Board interface {
    Name() string                      // "pi-zero-2w", "radxa-zero-3e"
    Artifacts() []ArtifactRef          // kernel, dtb, firmware, bootloader files (fetched/cached; stub with local paths until the artifact pipeline task lands)
    BootFiles(cfg BuildConfig, art Artifacts) (map[string]io.Reader, error) // FAT contents incl. kernel, initramfs, config.txt/cmdline.txt or extlinux/extlinux.conf
    RawWrites(art Artifacts) []image.RawWrite  // empty for Pi
    FirmwareFiles(art Artifacts) map[string]io.Reader // -> /lib/firmware/** in initramfs
}
```
Registry keyed by name; `--board` selects. Templates for config.txt/cmdline.txt/extlinux.conf live in internal/boards/<board>/ as go:embed text/template files — content specified by the two board epics; use placeholder templates until those land.

- [x] Board interface + registry + two skeleton boards
- [x] build command runs the full pipeline with a --artifacts-dir flag pointing at local kernel files for now
- [x] Integration test with fake artifacts: full build produces an image; read back with go-diskfs and assert kernel + initramfs + templates present

## Acceptance
`gosd build ./examples/hello --board=pi-zero-2w --artifacts-dir=./testdata/fake-artifacts -o /tmp/x.img` produces a structurally valid image with no network access.

## Summary of Changes

- `internal/boards.Board` is now the locked interface (`Name`, `Artifacts`,
  `BootFiles`, `RawWrites`, `FirmwareFiles`), with a `Register`/`Find`/`All`/
  `IDs` registry keyed by name. `internal/boards.ArtifactRef` +
  `ResolveArtifacts` resolve a board's declared artifacts by checking
  `--artifacts-dir` first and falling back to a pinned-URL fetch (via
  `internal/fetch`) into a persistent cache dir otherwise; a ref with no URL
  that isn't found locally (the pi-zero-2w kernel, until its build bean
  lands) is reported as an actionable "supply it via --artifacts-dir" error.
- `internal/boards/pizero2w` implements the real board profile: GPU boot
  firmware + config.txt/cmdline.txt (rendered from gosd-eu2x's locked
  templates) in `BootFiles`, and the WiFi firmware blobs plus their 8
  board-specific alias names (materialized as duplicate entries, not
  symlinks — the initramfs format doesn't carry those) in `FirmwareFiles`,
  both driven by `build/boards/pi-zero-2w/manifest.json` via a new embedded
  loader (`build/boards/pi-zero-2w/manifest.go`).
- `internal/boards/radxazero3e` is the registered skeleton: empty
  `Artifacts`/`RawWrites`, empty-map `FirmwareFiles`, and a `BootFiles` that
  fails clearly, pointing at bean gosd-gbsz.
- `internal/pipeline.Assemble` is the new build-pipeline orchestrator:
  resolves artifacts, builds firmware + config.json + the initramfs (in that
  order, since the initramfs must embed the firmware and is itself one of
  the board's boot files), calls the board for `BootFiles`/`RawWrites`, and
  writes the image via `internal/image.Write`. It replaces
  `internal/image`'s `Assembler`/`AssembleSpec`/`NotImplemented` stub, which
  is deleted along with `errNotImplemented`.
- `cmd/gosd build` gets a `--artifacts-dir` flag and now registers both
  boards and drives `pipeline.Assemble` per selected board; a
  `os.UserCacheDir()/gosd/artifacts` cache dir backs pinned-URL fetches when
  `--artifacts-dir` doesn't cover everything.
- `cmd/gosd-init/internal/initcfg` moved to `internal/initcfg` (imports
  updated in `cmd/gosd-init/main.go` and `cmd/gosd-init/internal/boot`), so
  `internal/pipeline` can build `/etc/gosd/config.json` from the same schema
  gosd-init parses, instead of duplicating it.
- New integration test (`cmd/gosd/build_integration_test.go`, with fake
  artifacts under `cmd/gosd/testdata/fake-artifacts/`) runs a full
  `gosd build ./examples/hello --board=pi-zero-2w --artifacts-dir=...`,
  reopens the resulting image with go-diskfs, and asserts the kernel,
  firmware, rendered templates, and initramfs (with /init, /app, firmware
  incl. aliases, and config.json) are all present. It also swaps in an
  `http.RoundTripper` that fails the test on any request, to positively
  assert the acceptance criterion's "no network access".
- Also added unit tests for the new `internal/boards`, `internal/boards/
  pizero2w`, `internal/boards/radxazero3e`, and `internal/pipeline` packages.

Deviations from the bean's sketch: the locked interface's `Artifacts`
parameter type wasn't fully specified, so it's implemented as a struct
holding resolved artifact paths (opened via `Open`/`MustOpen`) plus an
`Initramfs io.Reader` field the pipeline sets once the initramfs is built —
`FirmwareFiles`/`RawWrites` run before the initramfs exists (it embeds
`FirmwareFiles`' own output), so they see `Initramfs == nil`; only
`BootFiles` sees it populated.
