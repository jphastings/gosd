---
# gosd-vq4g
title: initramfs builder (pure-Go cpio + zstd)
status: todo
type: task
created_at: 2026-07-02T20:53:02Z
updated_at: 2026-07-02T20:53:02Z
parent: gosd-vi0n
blocked_by:
    - gosd-56xt
---

Build the initramfs archive in `internal/initramfs`, pure Go.

Contents (exactly this, nothing more): `/init` (gosd-init binary, mode 0755), `/app` (user binary, 0755), `/lib/firmware/**` (per-board firmware blobs, provided by the board profile), `/etc/gosd/config.json` (build-time config: hostname, wifi ssid/passphrase, board name — schema owned by gosd-init).

Use `github.com/u-root/u-root/pkg/cpio` (newc format) and `github.com/klauspost/compress/zstd` for compression. Directories must be explicitly present as cpio entries (kernels require parent dirs). Deterministic output: fixed mtimes (epoch 0), sorted entries — same inputs must produce byte-identical archives.

- [ ] API: `initramfs.Build(w io.Writer, spec Spec) error` where Spec lists files as (path, content reader, mode)
- [ ] zstd compression wrapper
- [ ] Unit test: build archive from testdata, decompress + parse with the same cpio package, assert exact entry list, modes and determinism (two builds byte-equal)

## Acceptance
`go test ./internal/initramfs` passes; archive from testdata inputs is byte-reproducible.
