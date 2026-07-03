---
# gosd-3zrc
title: Board profile abstraction and end-to-end build wiring
status: todo
type: task
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-02T20:53:02Z
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

- [ ] Board interface + registry + two skeleton boards
- [ ] build command runs the full pipeline with a --artifacts-dir flag pointing at local kernel files for now
- [ ] Integration test with fake artifacts: full build produces an image; read back with go-diskfs and assert kernel + initramfs + templates present

## Acceptance
`gosd build ./examples/hello --board=pi-zero-2w --artifacts-dir=./testdata/fake-artifacts -o /tmp/x.img` produces a structurally valid image with no network access.
