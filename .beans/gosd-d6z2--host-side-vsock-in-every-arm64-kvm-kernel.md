---
# gosd-d6z2
title: Host-side vsock in every arm64 KVM kernel
status: in-progress
type: task
created_at: 2026-11-01T09:21:46Z
updated_at: 2026-11-01T09:21:46Z
---

## Why

Tangled's Spindle microVM engine runs QEMU VMs on the board (ROCK 4SE first). On the host it needs `CONFIG_VSOCKETS` (it listens on an AF_VSOCK socket) and `CONFIG_VHOST_VSOCK` (QEMU opens `/dev/vhost-vsock`). The guest-side transport lives in the guest's own kernel, so physical boards don't need `VIRTIO_VSOCKETS`.

Released artifacts v0.10.4: pi-cm4, pi-3b and pi-zero-2w already have the full set, but only implicitly (bcm2711_defconfig `=m` promoted to `=y`), so a pin bump could drop it silently. Every mainline kernel (radxa-zero-3e, nanopi-zero2, rock-4se, cubie-a5e, turing-rk1, qemu-virt) has `# CONFIG_VSOCKETS is not set`.

## Locked decisions

- Every arm64 kernel gets the CM4's set: `VSOCKETS`, `VSOCKETS_DIAG`, `VSOCKETS_LOOPBACK`, `VHOST_VSOCK`, `VSOCKMON`. The hard requirements are `VSOCKETS` + `VHOST_VSOCK`; the rest match the CM4.
- qemu-virt also gets `VIRTIO_VSOCKETS` (guest transport, for local testing).
- pi-zero-w is out of scope: armv6, no KVM, nothing to host.
- Pi boards assert through their fragment (RequiredY is derived from it); mainline boards also list the symbols in their hand-maintained RequiredY.
- Snapshots are regenerated from a `workflow_dispatch` run of build-artifacts.yml on this branch, which also completes gosd-dcov.
- The `internal/artifacts.Version` bump is a separate, standalone follow-up PR after the artifacts release, not folded into gosd-6hdc.

## Todos

- [x] vsock block in the nine arm64 fragments (+ `VIRTIO_VSOCKETS` on qemu-virt)
- [x] Mainline RequiredY lists in internal/kernelspec
- [x] `artifacts:` change file
- [ ] Dispatch build-artifacts.yml on the branch; regenerate all ten kernel.config snapshots from its outputs
- [ ] Empty knownKernelConfigSnapshotDrift; complete gosd-dcov
- [ ] Follow-up: Version bump PR + COMPATIBILITY.md row, after the release
- [ ] ROCK 4SE bench: `/dev/kvm` + `/dev/vhost-vsock` exist, dmesg shows `kvm [1]: Hyp mode initialized`; record in docs/development/rock-4se.md
