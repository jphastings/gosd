---
# gosd-p4vl
title: Pin the vsock artifacts release; bench-check KVM on ROCK 4SE
status: todo
type: task
priority: normal
created_at: 2026-11-01T10:08:40Z
updated_at: 2026-11-01T10:08:40Z
blocked_by:
    - gosd-d6z2
---

## What

Once the artifacts release carrying gosd-d6z2's vsock kernels is published, pin it so `gosd build` images get vsock. This is a standalone bump, not folded into gosd-6hdc's CM4 activation, so the ROCK 4SE (Tangled Spindle's first target) isn't gated on CM4 work. gosd-6hdc can pin the same release later.

## Todos

- [ ] `build/artifacts/pin-bump.sh` to the new `artifacts/vX.Y.Z` (moves `Version` and `ManifestSHA256` together)
- [ ] `gosd:` change file: every arm64 board's image now has AF_VSOCK + `/dev/vhost-vsock`
- [ ] COMPATIBILITY.md feature row, e.g. "vsock for hosting VMs (`/dev/vhost-vsock`)": ✅ on every arm64 board, ➖ on Pi Zero W (no KVM)
- [ ] Three-way verification, recorded here: clean-HOME build of all public boards, offline re-run, released tarball's `kernel.config` shows `CONFIG_VHOST_VSOCK=y`
- [ ] ROCK 4SE bench: `/dev/kvm` and `/dev/vhost-vsock` exist, and dmesg shows `kvm [1]: Hyp mode initialized` (nVHE, since the A72/A53 are ARMv8.0), i.e. the board boots at EL2 through the pinned TF-A/U-Boot. Record in [the ROCK 4SE development notes](docs/development/rock-4se.md). If KVM doesn't initialize, open its own bean: it blocks Spindle regardless of vsock. This can run before the release with an `--artifacts-dir` image built from gosd-d6z2's dispatch run.
