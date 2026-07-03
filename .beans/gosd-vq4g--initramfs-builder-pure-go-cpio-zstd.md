---
# gosd-vq4g
title: initramfs builder (pure-Go cpio + zstd)
status: completed
type: task
priority: normal
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-03T17:10:17Z
parent: gosd-vi0n
blocked_by:
    - gosd-56xt
---

Build the initramfs archive in `internal/initramfs`, pure Go.

Contents (exactly this, nothing more): `/init` (gosd-init binary, mode 0755), `/app` (user binary, 0755), `/lib/firmware/**` (per-board firmware blobs, provided by the board profile), `/etc/gosd/config.json` (build-time config: hostname, wifi ssid/passphrase, board name — schema owned by gosd-init).

Use `github.com/u-root/u-root/pkg/cpio` (newc format) and `github.com/klauspost/compress/zstd` for compression. Directories must be explicitly present as cpio entries (kernels require parent dirs). Deterministic output: fixed mtimes (epoch 0), sorted entries — same inputs must produce byte-identical archives.

- [x] API: `initramfs.Build(w io.Writer, spec Spec) error` where Spec lists files as (path, content reader, mode)
- [x] zstd compression wrapper
- [x] Unit test: build archive from testdata, decompress + parse with the same cpio package, assert exact entry list, modes and determinism (two builds byte-equal)

## Acceptance
`go test ./internal/initramfs` passes; archive from testdata inputs is byte-reproducible.

## Summary of Changes

Implemented `internal/initramfs` for real, replacing the `NotImplemented` stub:

- `Spec` is now a flat list of `File{Path, Content io.Reader, Mode os.FileMode}`
  entries, per the bean's API shape, rather than the scaffold's board/path-based
  Spec.
- `Build(w io.Writer, spec Spec) error` writes a newc-format cpio archive
  (`github.com/u-root/u-root/pkg/cpio`) piped through a zstd encoder
  (`github.com/klauspost/compress/zstd`, single-goroutine encoder concurrency
  for reproducibility) directly to `w`.
- Parent directories are synthesized automatically from the given file paths
  (e.g. `/lib/firmware/wifi.bin` implies explicit `lib` and `lib/firmware`
  directory entries at mode 0755) — nothing beyond the listed files and their
  necessary ancestors is added.
- Determinism: every record has Ino/UID/GID/MTime left at zero (epoch 0), and
  all entries (files + synthesized dirs) are written in one global sorted-path
  order rather than relying on cpio's own per-file directory expansion, so two
  builds of an equivalent Spec are byte-identical.
- Added validation: duplicate paths and a path used as both a file and a
  directory are rejected with clear errors instead of silently overwriting.
- `internal/initramfs/initramfs_test.go` + `testdata/` fixtures build an
  archive, assert the exact 8-entry sorted list (4 files + 4 synthesized
  dirs), each entry's mode and content, MTime==0 for every entry, and that two
  builds of the same Spec are byte-equal. Also covers the duplicate-path and
  file/directory-collision error paths.

### Wiring into cmd/gosd

`internal/image.Spec.Initramfs` is still typed `initramfs.Builder` (owned by
another in-flight bean; not touched). To keep it compiling and to keep the
existing "not implemented" pipeline behavior intact:

- `Builder` is now `interface { Build(w io.Writer, spec Spec) error }` — same
  method the package-level `Build` func has — with a `DefaultBuilder{}` that
  delegates to it.
- `cmd/gosd/build.go` now wires `initramfs.DefaultBuilder{}` instead of
  `initramfs.NotImplemented{}` (which is deleted, along with its
  `errNotImplemented`, since the builder is no longer a stub).
- `image.NotImplemented.Assemble` never calls `Spec.Initramfs` yet, so the
  build pipeline's only remaining failure point is
  `"image assembly not implemented"` — verified via
  `go run ./cmd/gosd build ./examples/hello --board=pi-zero-2w -o /tmp/x.img`.

### Deviations

- The bean's stub had `Builder.Build(ctx, spec) (path string, err error)`
  writing to a file under `Spec.OutputDir`. Adapted it to the bean's actual
  required signature, `Build(w io.Writer, spec Spec) error`, which writes
  straight to a caller-supplied writer instead of a path — no context or
  output-directory concept needed at this layer. The image-assembly bean
  will decide where the archive's bytes end up (e.g. open a file and pass it
  as `w`).
- Directory mode is fixed at 0755; the bean didn't specify a value, and
  u-root's own `WriteRecordsAndDirs` helper defaults to 0777, which felt
  needlessly permissive for an embedded initramfs, so directories are
  synthesized directly instead of via that helper.
