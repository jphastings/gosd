---
# gosd-45bv
title: 'blockmount: concurrent emmc and disk FormatAndMount can both pick and format the same eMMC'
status: completed
type: bug
priority: normal
created_at: 2026-07-31T07:58:52Z
updated_at: 2026-08-02T11:13:16Z
---

Found by review sweep `gosd-fuxs` (storage area), verified (race window
certain; hitting it requires an app running both concurrently).

`blockmount.Run` is MountedAt → Discover → Inspect → Format → Mount with
no lock and no re-check between Discover and Format; both public packages
explicitly encourage backgrounded concurrent use ("returns immediately;
the work runs in the background"). On a Rockchip board booted from SD
with an idle onboard eMMC and nothing attached, disk's candidate list
(deviceClasses ends with "mmcblk") and emmc's (Kind == "MMC") are the
same single device; the InUse exclusion only helps after one side mounts.

**Failure scenario:** app starts `emmc.FormatAndMount("APPDATA",...)` and
`disk.FormatAndMount("BULK",...)` at boot (both doc-encouraged). Both
Discover before either Mounts → both format /dev/mmcblk0 with interleaved
writes → both mount it at two mountpoints with independent vfat
superblocks. Guaranteed corruption — but only when the interleaving lands
badly, so it's intermittent.

**Fix:** package-level mutex serialising blockmount.Run (once-per-boot,
slow ops; contention irrelevant) + a post-Discover re-check of
MountedSources immediately before Format.

## Summary of Changes

Added `runMu sync.Mutex` (package-level, internal/blockmount/blockmount.go)
held for the whole of `Run`: MountedAt through Mount, so a sibling call to
`Run` — from either `emmc` or `disk`, since both are thin wrappers over
this one function — cannot even start discovering until the current call has
mounted its device (making it show up in-use to the next Discover) or
failed. Added a `Deps.MountedSources func() (map[string]bool, error)` field,
called a second time immediately before `Format` runs; if the chosen device
now appears mounted, `Run` refuses instead of formatting. The two are
independent halves: the mutex only rules out a sibling `blockmount.Run` call,
the re-check also catches a device mounted by anything else (a udev rule,
another process) in the same window. Wired the real dependency in
emmc/platform_linux.go and disk/platform_linux.go (`blockmount.MountedSources`,
already existed); off-Linux stubs return `errUnsupportedPlatform` like their
siblings.

Tests (internal/blockmount/blockmount_test.go): a recheck test that scripts
a fake device becoming mounted between Discover and Format and asserts Run
refuses without formatting; a concurrency test that launches two Run calls
off a shared start barrier for the same fake device (standing in for emmc
and disk racing the same eMMC) and asserts exactly one formats and the other
is refused. Determinism comes from a gate inside the fake's Discover that
directly inspects the package's own `runMu` via `TryLock` (the test file
shares the package) rather than inferring serialisation from timing, so it
fails immediately and every time — not just when scheduling happens to
expose the bug — if the lock is ever missing; verified this by temporarily
disabling the mutex and the recheck in turn and confirming each test fails
deterministically before restoring the fix. Run under `-race` as a second,
independent check.
