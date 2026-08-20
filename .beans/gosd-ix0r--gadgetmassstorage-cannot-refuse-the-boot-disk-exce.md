---
# gosd-ix0r
title: 'gadget.MassStorage cannot refuse the boot disk except by accident: the mount-table check is the only guard'
status: todo
type: bug
created_at: 2026-08-20T06:01:15Z
updated_at: 2026-08-20T06:01:15Z
---

Split out of `gosd-cayj`, which fixed the example that took the sharp edge but
left the library edge itself in place.

`gadget.MassStorage.Create` refuses a `Path` that is currently mounted, is a
partition of a currently-mounted device, or is the parent of one
(`mountedAt`/`isPartitionOf`). That is the *only* protection against a caller
handing the boot disk to a USB host, and it is **incidental**: it works today
purely because gosd-init keeps `/boot` mounted for the life of the device. If
that ever stopped being true, `MassStorage{Path: "/dev/mmcblk0"}` would expose
the kernel and the whole config tree read-write — i.e. code execution on the
next boot.

## Why gosd-cayj did not just do it

There is no signal available to the `gadget` package today that identifies the
boot disk independently of the mount table:

- GoSD boots from an initramfs, so `/proc/cmdline`'s `root=` names no block
  device.
- The kernel marks no "this is the medium I booted from" attribute in sysfs
  portably across the fleet (SD, eMMC, virtio).
- So an "explicit" refusal written against `/proc/mounts` would be the same
  check `mountedAt` already performs, under a different name — no new
  guarantee, just the appearance of one.

## Shape a real fix probably takes

Have gosd-init **publish** what it booted from — the boot disk and the data
partition's device node — somewhere apps and libraries can read it, e.g. a
file under `/run/gosd/`, alongside the existing secret-registry file
(`internal/secretreg`). `gadget.MassStorage.Create` then refuses any `Path`
related to those devices whether or not anything is currently mounted, with
an actionable error. That makes the protection a rule the package enforces
rather than a side effect of another component's behaviour.

Worth deciding at the same time: whether the refusal should extend to the
data partition itself (which carries gosd-init's plaintext settings — see
`gosd-cayj`), or whether that stays an app-level judgement as it is in
`examples/usbwebsite` today.

## Todos

- [ ] Decide where gosd-init publishes the boot/data device identity (`/run/gosd/`?)
- [ ] Refuse those devices in `gadget.MassStorage.Create` regardless of mount state, with an actionable error
- [ ] Decide whether the data partition is refused outright or left to the app
- [ ] Tests covering the not-currently-mounted case, which is the whole point
