---
# gosd-d6z2
title: Host-side vsock in every arm64 KVM kernel
status: completed
type: task
priority: normal
created_at: 2026-11-01T09:21:46Z
updated_at: 2026-11-01T10:08:52Z
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
- [x] Dispatch build-artifacts.yml on the branch; regenerate all ten kernel.config snapshots from its outputs
- [x] Empty knownKernelConfigSnapshotDrift; complete gosd-dcov
- [x] Follow-up bean for the Version bump, COMPATIBILITY.md row and ROCK 4SE KVM bench check: gosd-p4vl

## Summary of Changes

- Added a vsock block to all nine arm64 kernel fragments (pi-cm4, pi-3b, pi-zero-2w, radxa-zero-3e, nanopi-zero2, rock-4se, cubie-a5e, turing-rk1, qemu-virt) with `VSOCKETS`, `VSOCKETS_DIAG`, `VSOCKETS_LOOPBACK`, `VHOST_VSOCK` and `VSOCKMON`; qemu-virt also gets `VIRTIO_VSOCKETS`. The six mainline boards' hand-maintained RequiredY lists carry the same symbols; the Pi boards' RequiredY derives from the fragment.
- `workflow_dispatch` run 41293550798 of build-artifacts.yml built all ten kernels green on this branch, so every new symbol survived olddefconfig. `strings` on the rock-4se `Image` and the pi-3b `kernel8.img` shows `vhost-vsock`.
- Regenerated all ten `kernel.config` snapshots from that run's uploads and emptied `knownKernelConfigSnapshotDrift`, completing gosd-dcov.
- `artifacts: minor` change file. The pin bump and the ROCK 4SE KVM check are gosd-p4vl.
