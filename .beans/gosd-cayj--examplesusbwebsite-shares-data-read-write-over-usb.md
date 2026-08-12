---
# gosd-cayj
title: examples/usbwebsite shares /data read-write over USB, exposing the WiFi PSK and ingress tokens to any host
status: todo
type: bug
created_at: 2026-08-12T04:15:09Z
updated_at: 2026-08-12T04:15:09Z
---

**Severity: High.** GoSD's own reference example hands plaintext credentials
and write access to anyone who plugs in a USB cable.

## Verified

- `examples/usbwebsite/main.go:337` —
  `gadget.MassStorage{Path: st.device, Removable: true}`. **`ReadOnly` is
  not set.**
- `gadget/massstorage.go:27-28` — `ReadOnly bool`, zero value `false`. The
  package's own `TestMassStorageFlagsDefaultOff` confirms the default is
  off. So an omitted field means read-write, silently.
- `st.device` is the **data partition** (`main.go:242-256`).
- `cmd/gosd-init/internal/provsnapshot/provsnapshot.go:648` renders the
  effective gosd.toml — WiFi passphrase and any Cloudflare/Tailscale token
  included — onto `/data` on every boot. The package's own comment
  (`:645-646`) acknowledges it holds the tunnel token "the same trust level
  as the WiFi passphrase already stored here in plain text."

The example targets eMMC-less boards (the Pi Zero W family) explicitly, so
this is the path those boards take, not an exotic configuration.

## Attack

Plug the running device into any computer. It enumerates as a removable
mass-storage device. Mount it — FAT has no hidden-file concept, and a macOS
or Windows host shows `.gosd/` plainly. Read
`.gosd/provision-snapshot/gosd.toml`: the WiFi PSK for the network the
device is on, and any tunnel token, in full.

Then write to it. Because the share is read-write, the same USB cable lets
an attacker plant a provisioning snapshot — which, being unauthenticated
(sibling bean), survives the owner's reflash and re-provisions the device.
The two findings chain into persistence that outlives the owner's cleanup.

No case to open, no card to remove, no credentials required.

## Fix

1. `examples/usbwebsite`: set `ReadOnly: true` for the eMMC-less data-partition
   share, or point the share at a dedicated subdirectory image rather than
   the partition that holds the snapshot.
2. Consider inverting the `gadget.MassStorage` default so write access is
   opt-in (`Writable bool`) rather than opt-out. A field whose zero value
   grants an unauthenticated host write access to a block device is the
   wrong default for this library, and the example proves the default gets
   taken by accident.
3. Document the interaction in `docs/runtime.md` beside `gadget.MassStorage`:
   sharing the data partition shares the provisioning snapshot's secrets.
4. Add an explicit refusal in `gadget/massstorage.go` for the disk backing
   `/` or `/boot`. Today the only protection is `mountedAt`/`isPartitionOf`
   (`:91-150`), which blocks a **currently mounted** path — it is incidental
   that gosd-init keeps /boot mounted, not a rule the package enforces. If
   that ever changes, `MassStorage{Path: "/dev/mmcblk0"}` exposes the kernel
   and gosd.toml read-write, i.e. code execution on next boot.

## Todos

- [ ] `ReadOnly: true` (or a non-secret-bearing backing store) in examples/usbwebsite
- [ ] Decide on inverting the MassStorage write default; if kept, document the zero value loudly on the field
- [ ] Refuse the boot/root disk explicitly in gadget/massstorage.go
- [ ] docs/runtime.md: call out that sharing /data shares the snapshot's plaintext secrets
