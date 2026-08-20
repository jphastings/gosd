---
# gosd-jtm1
title: 'disk: FormatAndMount gives up on a USB drive that is still enumerating'
status: completed
type: bug
priority: normal
created_at: 2026-08-19T10:35:03Z
updated_at: 2026-08-19T10:44:44Z
---

`disk.FormatAndMount` discovers exactly once. `discover()` (disk/platform_linux.go)
takes a single snapshot of `/sys/block`, and `blockmount.Run` calls it one time —
there is no settle, no retry, nowhere in the path.

That is harmless for the storage classes GoSD was first built around: NVMe and
eMMC sit on on-SoC buses and are enumerated before userspace runs. USB mass
storage is not like that. A stick or enclosure needs its hub port powered, then
probe, then the SCSI scan, then "medium ready" — commonly 1-3s after the host
controller comes up, and longer through a hub or for a spinning enclosure. An
app that calls `FormatAndMount` early in `main()` therefore gets `ErrNoDisk` for
a drive that is physically plugged in and appears a moment later. The error is
actively misleading: `ErrNoDisk` is documented as "nothing suitable is
attached", and here something suitable *is* attached.

True hotplug — a drive plugged in minutes after boot — is the same gap seen from
further away, and is why the fix must not be a fixed internal constant.

## Locked decisions

- **Add `disk.Options.Wait time.Duration`**: a bounded wait for a candidate to
  appear before discovery gives up. **The zero value is today's behaviour
  exactly** (discover once, no wait). Not a default settle window: an app that
  treats `ErrNoDisk` as "no disk here, carry on" would be stalled by a default,
  and a board with nothing ever attached would pay the full window on every
  boot. The caller knows which of the three cases they are in (fast boot, slow
  enclosure, or genuine later hotplug) and picks the duration; GoSD does not
  guess. No signature changes, so no semver break.
- **The wait happens BEFORE `blockmount.Run`, never inside its `Discover`.**
  `Run` holds the process-wide `runMu` across discovery, and that lock is shared
  with the `emmc` package (see its doc comment, gosd-45bv) — waiting inside
  `Discover` would block a sibling `emmc.FormatAndMount` for the entire window.
  Waiting outside leaves the serialisation argument completely untouched: `Run`
  still performs its own discovery under the lock, and if the device disappears
  between the wait and the lock it reports `ErrNoDisk` exactly as it does today.
  The wait only ever answers "has a candidate shown up yet?" — it never resolves
  the device that gets formatted.
- **`Options.Wait` composes with `Options.Device`**: waiting on a named device
  polls until that device is present and not in use, so an app that knows it
  wants `/dev/sda` can wait for precisely that.
- **`ErrNoDisk`'s message names the option** when `Wait` was zero, per the
  actionable-errors rule — a USB drive still enumerating is the most likely
  cause of a surprising `ErrNoDisk`, and the remedy is one field.

## Todos

- [x] `Options.Wait` + a pure, fake-driven wait helper in `disk` (poll interval 250ms)
- [x] Wire it in `FormatAndMountWith` ahead of `blockmount.Run`
- [x] `ErrNoDisk` message points at `Options.Wait` when it was unset
- [x] Behavioural tests: appears-late succeeds, never-appears reports `ErrNoDisk` after the window, zero-Wait does not poll, named-device wait
- [x] `docs/runtime.md` "Attached disk storage" documents the USB enumeration delay and `Options.Wait`
- [x] Change file

## Summary of Changes

`disk.Options.Wait` (`disk/disk.go`) is a bounded wait for a disk that has not
appeared yet. Its zero value probes once, so no app changes behaviour by
upgrading — the option only ever adds patience that was not previously
expressible.

Three small functions carry it, all fake-driven in `disk/disk_test.go`:

- `awaitStorage(deps, probe, mountpoint, wait, sleep)` decides whether waiting
  is even the right thing to do. A zero `Wait` returns immediately, so an app
  that did not ask for the option does not pay even one extra `/sys/block` read
  for it. **It also refuses to wait when the mountpoint is already mounted** —
  found while reviewing the first draft of this change, which got it wrong.
  `blockmount.Run` short-circuits a warm restart (app relaunched without a
  reboot) *before* it discovers anything, and a mounted disk is deliberately
  never a discovery candidate, so an unconditional pre-`Run` probe would have
  spent the whole window and then reported `ErrNoDisk` for a disk that was
  mounted and working — a regression at any non-zero `Wait`. Both that case and
  the unreadable-mount-table case are pinned by tests, verified by removing the
  guard and watching them fail.

- `awaitCandidate(probe, wait, sleep)` polls until a candidate appears or the
  window closes, with `sleep` injected so the tests drive the whole schedule
  without spending the wall-clock time. It retries **only** `ErrNoDisk` — the
  one error that means "not there (yet)", including a present-but-in-use disk
  that may be released. An unreadable `/sys/block`, or a named device something
  else has mounted, returns immediately rather than after the full window,
  because waiting cannot change either.
- `explainNoDisk(err, waited)` adds the remedy: an app that never asked to wait
  is told the option exists, one that did is told how long it waited so the
  number can be raised knowingly. It wraps with `%w`, so existing
  `errors.Is(err, ErrNoDisk)` callers see no change, and passes every other
  error through untouched.

The wait runs in `FormatAndMountWith` **before** `blockmount.Run`, as the bean
locked. `Run` holds a process-wide lock across discovery that it shares with
`emmc` (gosd-45bv), so waiting inside `deps.Discover` would have stalled a
sibling `emmc.FormatAndMount` for the entire window. Waiting outside leaves that
serialisation argument untouched: `Run` still discovers for itself under the
lock, and a drive that appears and is then claimed by something else in between
still reports `ErrNoDisk`. The wait never resolves the device that gets
formatted — it only answers "has one shown up yet?", so the boot-media
exclusion, the class allowlist and the in-use rule all still apply exactly as
before.

`Options.Wait` composes with `Options.Device`: the probe becomes
`verifyNamedDevice`, so an app that knows it wants `/dev/sda` waits for
precisely that.

Docs: the attached-disk section of the runtime guide gains the USB enumeration
delay and a worked `Wait` example, plus the reasoning for there being no default
window; `ErrNoDisk`'s and the package's own doc comments now say that a slow USB
drive is indistinguishable from an absent one and point at the option.

### Not done here

Nothing in the discovery path is exercised on real USB hardware by this change —
the tests are host-side and behavioural, and CI's `qemu-disk-ext4` job uses
virtio disks that are present at boot, so the late-appearance path it fixes has
no automated on-device coverage. Verifying it needs a bench pass with a real
stick (and ideally a slow enclosure behind a hub); worth a follow-up bean rather
than a claim here.
