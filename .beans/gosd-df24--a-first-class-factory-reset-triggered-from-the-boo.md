---
# gosd-df24
title: A first-class factory reset, triggered from the boot partition
status: todo
type: feature
priority: high
created_at: 2026-08-20T08:36:08Z
updated_at: 2026-08-20T08:36:08Z
---

**Why this exists.** Bean `gosd-7m9y` established that `/data` is a trust
boundary a reflash does not cross, and that the config store cannot be
authenticated on these boards (no key can live anywhere an attacker can't
also read — see that bean's Summary of Changes for the full argument). JP's
decision of 2026-08-20 accepted that residual risk rather than narrowing what
the store keeps: a reflash must put back **all** of a device's settings,
credentials included, because surviving a reflash is the store's whole
purpose. **This bean is the compensating control** — the thing that makes the
remedy the documentation now advises actually performable.

Today that advice is "clear or reformat the data partition", and for most
owners it is not an available operation:

- On an ext4 `/data` (`gosd build --data-filesystem ext4`) macOS and Windows
  can neither read it nor clear it. That is the majority of owners.
- Even on FAT32 `/data`, "delete the hidden `.gosd` directory from the second
  partition" is a level of instruction that gets performed wrong or not at
  all.

So the only partition an owner can reliably edit — from any OS, with any
flasher, with the card in a USB reader — is the **boot** partition, which is
FAT by construction. That is where the trigger has to live.

## Shape

A config-tree value, e.g. `config/factory_reset`, so it inherits the whole
apparatus `internal/configtree` already enforces:

- A mandatory `.explain.md` sidecar (`configtree.DocSuffix`), which is where
  the destructive consequence gets spelled out for the person holding the
  card. Required at build, never at runtime.
- The padding/reservation rules: shipped padded to at least
  `configtree.MinValueBytes`, and the reservation is fixed at build time
  because gosd-init overwrites the file **in place** on a mounted FAT
  partition. Every form this file can hold — armed and consumed alike — must
  fit the shipped reservation.
- Empty means unset, which is what every image ships.

**Hard to trigger by accident.** Not a boolean: a `1` or a `true` is one
stray keystroke, and this destroys data. Proposal: the file must contain the
device's own hostname (`initcfg.Config.Hostname`, which gosd-init already
knows), the way a "type the repository name to delete it" confirmation
works — self-documenting, impossible to hit by accident, and unambiguous
about *which* device is being reset when somebody has several cards on a
desk. The `.explain.md` states the exact expected string verbatim. Anything
else in the file — blank, junk, an editor's stray whitespace, the wrong
hostname — is a no-op that logs what it saw and what it expected.

## Crash ordering — the trigger is consumed before anything is destroyed

The failure this must not have is a loop: reformat `/data`, fail to clear
the trigger, reformat again on every subsequent boot. Two orderings were
weighed and only one survives.

**Rejected: reformat first, clear afterwards.** A trigger that outlives its
own reformat is armed again next boot. If clearing fails *persistently* —
a read-only boot partition is a state gosd-init already handles — the device
destroys `/data` on every boot forever, which is precisely the loop this
must not have. No amount of retry makes that converge.

**Rejected: a completion receipt written into the new `/data`.** It looks
like the codebase's marker discipline but is circular: the receipt has to
distinguish "the reset I already performed" from "a second reset the owner
has since asked for", and both requests carry identical bytes on identical
partitions. Any key it could use lives on the side being erased.

**Locked: the consumption is written first, durably, on the boot partition,
and nothing is destroyed until it is.** Stated as a rule:

> gosd never destroys `/data` unless it has already durably recorded that it
> is doing so.

Concretely, three states in the one file, each written with this codebase's
write → sync → marker → sync discipline (`cmd/gosd-init/internal/durable`,
fsyncing the file *and* its directory, since the boot partition is FAT and
rename atomicity cannot be assumed):

1. **Armed** — the hostname, typed by the owner.
2. **In progress** — written and synced *before* the first destructive act.
   A crash here leaves `/data` in an unknown state, so the next boot finishes
   the job: it treats "in progress" as a second arming and reformats again.
3. **Done** — written and synced after the reformat is established.

That loop cannot destroy anything the owner wanted: the reset runs before the
app starts, so on any boot where "in progress" is set there is by
construction no new data on `/data` to lose. It converges the moment a boot
gets far enough to write "done".

And if the boot partition cannot be written at all, **the reset is refused
and logged, never performed** — because a reset gosd can't record is a reset
that repeats forever.

## Ordering against the rest of the boot

- **Before `cmd/gosd-init/internal/configstore`'s restore.** The store lives
  on `/data`. Reset after it and the settings the owner asked to destroy are
  already back on the card.
- **Consequence worth writing down, because it means no new exclusion is
  needed:** by the time the store's persist pass runs, the trigger file
  already reads a consumed form, so that is what the store keeps and later
  restores — and a consumed form is inert. The *armed* form is therefore
  never kept. Verify this rather than assume it; if the ordering ever slips,
  the store would restore a live "erase my data" command onto a freshly
  flashed card.
- **Against `cmd/gosd-init/internal/dataexpand`**, which runs before the
  normal `/data` mount and gates on the MBR entry, the establishment marker
  and a case-insensitive match against *this image's* data label. Preferred
  implementation is to **reuse that path rather than write a second
  formatter**: invalidate the existing volume (clear the establishment
  marker, and for an `--data-size=expand` image the partition-2 MBR entry
  too) so dataexpand's existing "no entry / unmarked / foreign volume"
  branch does the destroying, with its crash argument already proven. A
  reflash reaches that same state today, which is the precedent.
- **The adoption gate** is what makes this correct rather than lucky: after
  the reset, the volume must be re-established from `config.json`'s baked
  `dataFilesystem` and `dataLabel` (`initcfg.Config`), never from a probe of
  what happens to be on the partition. A card whose previous image used a
  different label or filesystem is reformatted to *this* image's on-card ABI,
  exactly as the adoption gate already does after a `--label-prefix` or
  `--data-filesystem` change.
- **ext4 must re-run the grow**, not assume the partition is already the
  right size: a fixed-size ext4 image ships `diskfmt`'s ~512MiB golden and
  grows once via `EXT4_IOC_RESIZE_FS` at establishment, so a reset that
  reformats has to establish again, not adopt.

## Logging

Loud, on the console, and on both sides of the act: before, naming the
partition, its label, its filesystem and that everything on it is about to be
destroyed; after, saying it is done and that the device is now running the
settings the card itself carries. A refusal says what it read and what it
expected. Nothing ever logs a value the store held.

## Open design points (decide in the PR, record here)

- **What happens when the reformat itself fails midway.** Continuing to boot
  the app on the old `/data` means the owner's remediation silently did not
  happen; halting bricks a device over a settings problem. Leaning towards:
  leave the trigger in its "in progress" form so the next boot retries, log
  loudly, and reserve `fault.Fatal` for the case where `/data` is left in a
  state nothing can safely mount.
- Whether the consumed forms are also surfaced through a status LED pattern
  (`internal/statusled`), since a headless owner has no console.

## Non-goals

- **No network or remote trigger.** gosd-init has no interactive surface and
  this does not become the exception.
- **No kernel cmdline trigger** (`gosd.*`). The point is a partition an owner
  can edit from the OS they already have; cmdline editing is a strictly
  smaller audience and a second armed path to maintain.
- **Not a boot-partition reset.** Reflashing already resets the boot
  partition, and does it better.

## Todos

- [ ] Decide the trigger's exact filename, armed spelling and consumed forms; write the `.explain.md`
- [ ] Ship the value in gosd's own defaults tree with its reservation
- [ ] Implement consumption (armed → in-progress → done) with the durable write discipline, refusing the reset outright when the boot partition can't be written
- [ ] Wire the destroy through dataexpand's existing establishment path, before the config store's restore
- [ ] Re-establish from config.json's baked dataFilesystem/dataLabel, including the ext4 grow
- [ ] Test: a crash between "in progress" and a completed reformat reformats again next boot and then converges
- [ ] Test: an unwritable boot partition refuses the reset rather than looping
- [ ] Test: the armed form is never what the config store keeps
- [ ] Test: a wrong/blank/junk trigger value is an inert no-op
- [ ] qemu-virt end-to-end: settings and credentials present, reset, reboot, card's own settings only
- [ ] Document in docs/config.md, replacing "clear or reformat the data partition" as the advised remedy, and update the trust-boundary sections in configstore's package doc and docs/design/upgrade-path.md §3a
