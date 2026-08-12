---
# gosd-psj0
title: An unmountable volume must not be reformatted without consent
status: completed
type: bug
priority: high
created_at: 2026-08-09T05:47:13Z
updated_at: 2026-08-09T05:55:07Z
---

JP, 2026-08-09. gosd-1c0x left one path in runEXT4 that destroys data with no consent, flagged at the time as a residual gap to revisit 'if it ever proves wrong in practice'. It has.

In runEXT4's label-and-filesystem-match branch, every consent check — the establishment marker, RootHasOtherContent, and the destructive gate — lives inside the mount-SUCCESS arm. When Mount fails outright, all of it is skipped and control falls through to Format with no reference to destructive at all. So a volume carrying the app's own label and filesystem, holding real data, that has become unmountable through corruption or an unrelated hardware fault, is silently wiped on the next boot.

The original reasoning was that an unmountable filesystem's root can't be read, so the RootHasOtherContent second opinion can't be applied, and telling 'corrupt debris' from 'corrupt-but-real data' is materially harder. True — but the conclusion doesn't follow. When we cannot tell the two apart, the safe default is to refuse, not to wipe. GoSD boards have no shell, so an unwanted format is unrecoverable, whereas a refusal is one config edit away from resolution: ErrRefusedFormat plus an app-level env var is an ergonomic, proven path (atfs's ATFS_FORMAT_DRIVES_IF_NOT_ATFS does exactly this).

The rule JP wants: **without destructive, nothing that might hold data is erased.** Only a provably-empty device is formatted unasked.

Found via atfs bean ATFS-c2ny, which was mirroring its libp2p identity key onto a second volume specifically to survive this path. The identity is unrecoverable — an atfs instance's atproto config record is keyed by its peer ID — which is what makes a silent wipe expensive rather than merely annoying.

## Scope

- [x] runEXT4: when Mount fails in the !format branch, refuse with ErrRefusedFormat unless destructive; reformat when destructive, with action = reformatting
- [x] Keep the no-marker + empty-root self-heal exactly as it is. That case is provably safe rather than merely probably safe: an app can only write to the mountpoint after Run has handed it back, which never happened, so an empty root cannot be data. Requiring consent there would brick a device whose first format was interrupted, for no gain
- [x] Blank devices are unaffected — Blank never enters the !format branch
- [x] Tests: an unmountable matching-label volume refuses without destructive, and reformats with it
- [x] runEXT4's doc comment currently explains why the gap is accepted; rewrite it to state the rule instead
- [x] Both disk and emmc inherit this via internal/blockmount — check neither has a test asserting the old behaviour

## Summary of Changes

`runEXT4`'s mount-failure arm (the `else` of `if mountErr := s.Deps.Mount(...); mountErr == nil`) no longer falls through to an unconditional reformat. It now branches on `destructive`: false returns an error wrapping `ErrRefusedFormat` (naming the label/filesystem found, the mount error, and that the data may be unreadable-but-real), leaving the device untouched; true sets `action = "reformatting"` and proceeds, matching the other overwrite paths' wording. No `Unmount` call was added — nothing was mounted, so there is nothing to release.

Both `runEXT4`'s doc comment and the inline comment that used to justify the unconditional reformat were rewritten to state the rule (refuse when debris can't be told from real data) instead of explaining why the old gap was accepted. The marker-insufficiency and `RootHasOtherContent` reasoning in the no-marker branch was left as-is — still correct, still load-bearing.

Confirmed unaffected: the no-marker+empty-root self-heal (untouched code path); blank devices, which never reach `runEXT4`'s `!format` branch at all (`Run`'s switch only sets `format=false` when `contents.FS == fs`, and `Contents.Blank` always carries `FS == ""`) — already exercised by the existing `TestRunEXT4FreshFormatEstablishesInOrder`/`TestRunEXT4GrowFailureSurfacesActionableErrorAndSkipsMarker`, which format blank ext4 media with `destructive=false`; and `disk`/`emmc`, whose own test suites don't exercise this branch at all (no mount-failure scripting in either), so neither encoded the old behaviour.

`TestRunEXT4MountFailureOnAdoptionAttemptReformats` DID encode the old (buggy) behaviour — it asserted that a mount failure on a label/filesystem-matching device reformatted unconditionally with `destructive=false`. Renamed/split into `TestRunEXT4MountFailureOnAdoptionAttemptRefusesWithoutDestructive` (asserts `ErrRefusedFormat`, no `Format`/`Unmount` call) and `TestRunEXT4MountFailureOnAdoptionAttemptReformatsWhenDestructive` (asserts the reformat proceeds, still with no spurious `Unmount`, when `destructive=true`).
