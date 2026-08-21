---
# gosd-ix0r
title: 'gadget.MassStorage cannot refuse the boot disk except by accident: the mount-table check is the only guard'
status: todo
type: bug
priority: normal
created_at: 2026-08-20T06:01:15Z
updated_at: 2026-08-21T08:04:48Z
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

- [x] Decide where gosd-init publishes the boot/data device identity (`/run/gosd/`?) — `/run/gosd/reserved-devices.json`, format owned by `internal/devreserve`
- [x] Refuse those devices in `gadget.MassStorage.Create` regardless of mount state, with an actionable error — wrapping the new `gadget.ErrReservedDevice`
- [x] Decide whether the data partition is refused outright or left to the app — **left to the app**; reasoning below
- [x] Tests covering the not-currently-mounted case, which is the whole point


## Decisions

**Where it is published: `/run/gosd/reserved-devices.json`, with the format
owned by `internal/devreserve`.** It sits beside the secrets registration
file and the fault drop file, and follows their discipline — one small
package defines the bytes and both ends use it, write-then-rename with no
fsync (tmpfs buys no durability), named optional fields because the two ends
are not necessarily the same release. gosd-init writes it the moment the
boot partition mounts, which is the first moment it knows the answer and is
comfortably before `/app` starts.

**What it publishes is a list of *devices GoSD reserves*, not a description
of the card.** One entry per device node, each carrying prose the reader
quotes back and never interprets. That prose is what lets a gosd-init newer
than an app's compiled-in `gadget` package reserve a device class the app
has never heard of and still get a refusal its owner can act on.

**The containment rule runs one way, and that is the whole trick.** A
candidate is refused when it would *contain* a reserved device: the device
itself, or the whole disk a reserved partition sits on. So reserving
`/dev/mmcblk0p1` refuses both `/dev/mmcblk0p1` and `/dev/mmcblk0` — which is
the attack in the bean's opening paragraph — while leaving `/dev/mmcblk0p2`
alone. That is what keeps the published list minimal (no whole-disk entry is
needed, and none is written) and is why a third device class costs one line.

**The data partition is NOT refused — left to the app.** Three reasons:

1. It is the app's own persistent storage. `disk`/`emmc` hand an app a
   `BlockDevice` precisely so it can be published; `/data` is the same kind
   of thing on a board with no eMMC.
2. Refusing it would reverse `gosd-4ajn`'s locked intent (JP, 2026-07-25) —
   the SD data partition exists as the mass-storage vehicle for the
   eMMC-less Pi Zeros, whose bring-up is still outstanding — and would break
   `examples/usbwebsite`'s documented `WEBSITE_SHARE_DATA` path, which
   `gosd-cayj` deliberately shipped as a consent gate rather than a refusal.
3. It would be treating a symptom. What is on `/data` that should not be
   published is gosd-init's config store, and the structural answer is
   `gosd-onjv` moving that store to a partition of its own. Reserving *that*
   partition is correct and narrow; reserving the app's whole storage volume
   because gosd-init happens to keep secrets on it is not.

The refusal is therefore a floor (never the boot medium), not a review of
the app's choice of volume — and `docs/runtime.md` says so, immediately
above the unchanged "do not share the data partition" guidance.

**Fail open on absence, fail closed on corruption.** A missing file means no
reservations, because an app's `gadget` package can be newer than the
gosd-init that built its image (the app compiles gosd from its own go.mod;
gosd-init is built by whichever gosd CLI ran the build), and refusing to
work on an image that is no less safe than it was last week would be a
functional regression with a baffling symptom. A file that IS present but
cannot be read or parsed is reported instead: `devreserve.Encode` cannot
produce one, so it means something other than gosd-init wrote `/run/gosd`,
and carrying on would mean not knowing what is reserved. This is a
deliberate departure from `faultdrop`/`secretreg`'s drop-it-wholesale rule,
where ignoring a bad file costs a crash report rather than the refusal
itself.

**A new sentinel, `gadget.ErrReservedDevice`.** `gadget` is public API and
this is a new way for `Apply` to fail, so a caller that can do something
else — offer a different volume, run without the drive — gets to match it
with `errors.Is`, the way `gadget.ErrNoController` already allows.

## How `gosd-onjv` plugs in

One line, in `cmd/gosd-init/internal/boot/reserve.go`: append a second
`devreserve.Entry` for the config partition's device node, with its own role
prose. Nothing in the `gadget` package changes, and **already-compiled apps
refuse it too** — the reader never enumerates known device classes, it only
asks "would this LUN expose anything on the published list". The
containment rule already covers the whole-disk case, so the config
partition does not need its own disk entry either.

The one thing worth deciding there rather than inheriting: `reserveDevices`
is called right after the boot mount, so a config partition mounted later in
the sequence should either be reserved from its known device node at that
same point (preferred — the node is derivable from the boot device, and an
app can read the list the moment it starts) or the publish moved to after
its mount, at the cost of a window in which the list is incomplete.

## Deferred, with a bean

`internal/blockmount.Candidates` relies on the identical accident — its own
comment says the mount table "is what keeps the media the board booted from
off the list" — and there the consequence is a **format**, not a
disclosure, since `disk.FormatAndMount`/`emmc.FormatAndMount` choose their
target through it. Out of scope here (it changes format-target selection,
which needs its own destructive-operation reasoning), filed as `gosd-zldw`,
and it can consume this bean's mechanism unchanged.

## Summary of Changes

- **`internal/devreserve` (new)** — the reserved-device file: `Path`
  (`/run/gosd/reserved-devices.json`), `Entry{Path, Role}`, `Encode`/`Parse`,
  `Read`/`Write` (write-then-rename, no fsync, mode 0644), and the
  `Covers`/`Reservations.Exposes` containment rule. `isPartitionOf` moved
  here from `gadget` so the mounted-device check and the reserved-device
  check can never disagree about Linux's partition-naming convention;
  `path.Clean` is now applied to both sides. A role that isn't valid UTF-8
  or carries control characters is dropped while its entry's *path* is kept,
  so a bad publisher loses the explanation and never the refusal.
- **`gadget/massstorage.go`** — `Create` consults the list before the mount
  check (the reservation is the fact that survives an `Unmount`, so it is
  the one worth reporting), with two distinct actionable errors for "this is
  the device" and "this is the disk holding it", both wrapping the new
  `ErrReservedDevice`. `relatedDevicePaths` now delegates to
  `devreserve.Covers`. New `reservedDevices` test seam mirroring the
  existing `mountedTargets` one.
- **`cmd/gosd-init/internal/boot/reserve.go` (new)** + `Deps.ReserveDevices`
  — publishes the boot-partition device the boot mount actually used, right
  after that mount and before `/app`. Failure is logged and never fatal: an
  app then has the mounted-device check it always had, and halting a device
  over a tmpfs write would be the worse outcome. `cmd/gosd-init/main.go`
  wires it to `devreserve.Write`.
- **Tests** — the not-currently-mounted case is covered from both ends:
  `TestMassStorageRefusesAReservedDeviceWhileNothingIsMounted` (boot
  partition, whole card, and a device class this package has never heard of,
  proving the `gosd-onjv` extension point) and
  `TestMassStorageAllowsTheDataPartitionOfAReservedDisk`, plus
  precedence-over-the-mount-check, the documented fail-open, and the
  read-error path. `TestReservedBootPartitionRefusesTheWholeCard` pins that
  the two halves agree, since only a shared file couples them —
  publishing the mountpoint, or the whole disk instead of the partition,
  would pass every other test and get the app-side answer wrong.
- **`docs/runtime.md`** — one bullet in the USB gadget section, above the
  unchanged data-partition guidance.
- **`.changeset/gadget-refuses-the-boot-partition.md`** — `gosd: minor`.

**Verified:** `go test ./...`, `go vet ./...`, `gofmt -l .` (clean),
`golangci-lint run ./...` and `GOOS=linux golangci-lint run ./...`, plus
`GOOS=linux GOARCH=arm64 go build ./examples/...` and
`GOOS=linux GOARCH=arm GOARM=6 go build ./examples/... ./sound`.
