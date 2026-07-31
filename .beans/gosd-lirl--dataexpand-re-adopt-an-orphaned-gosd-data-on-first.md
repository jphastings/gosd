---
# gosd-lirl
title: 'dataexpand: re-adopt an orphaned GOSD-DATA on first boot after a reflash'
status: completed
type: feature
priority: normal
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-07-31T10:25:45Z
---

Phase 1 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §2). First, derive the data-partition offset from the flashed MBR (partition 1 start + size) and DELETE dataexpand's mirrored dataPartitionStartLBA constant — with per-app boot sizes (design §0.4) a mirror is wrong by construction. Then insert an Inspect between AddKernelPartition and FormatFAT32 in dataexpand.Run: a FAT32 volume labelled GOSD-DATA at that derived offset is adopted (skip format), anything else formats fresh as today. MBR write stays the commit record; power-loss analysis unchanged. Behavioral tests: reflash-then-boot adopts and preserves contents; foreign/blank content still formats; interrupted adoption redoes cleanly. Applies to --data-size=expand images only — the docs bean covers saying so.

## Summary of Changes

`cmd/gosd-init/internal/dataexpand` no longer knows where the data
partition starts. `dataStartLBA(mbr)` reads partition 1's entry and returns
`start + size` — the sector the image writer left free — and the mirrored
`dataPartitionStartLBA` constant is deleted. `bootPartitionStartLBA` (16MiB)
stays: partition 1's *start* is still locked, only its size becomes per-app.
`partitionSectors` now takes that derived start, and `checkGosdMBR` gained
one guard: a partition 1 of zero length (or one ending past the MBR's
uint32 sector range) is refused as a foreign table rather than deriving a
data offset on top of the boot partition.

Re-adoption sits between the kernel-partition registration and the format:
`survivorPresent` inspects the new partition node and, when it holds FAT32
labelled GOSD-DATA *and* the completion marker below, both `FormatFAT32` and
`SyncDevice` are skipped and the run logs "data partition re-adopted, its
contents intact" instead of "data partition created, filling the card".
Anything else — blank, foreign label, exFAT, unrecognisable mid-filesystem
rubble — formats exactly as before. An Inspect *error* is neither: it aborts
the boot's expansion (read-only /data, retried next boot) rather than
formatting over contents that could not be seen.

**Format-completion marker (added after code review found a data-corruption
hole in the first implementation).** Inspecting FAT32 + the GOSD-DATA label
is NOT proof of a finished format: go-diskfs's `Create` writes boot sector →
FATs → root directory → `SetLabel` with no sync between them, and Inspect's
label comes from the root directory's volume-label entry. A power cut during
a first boot's own format can therefore persist the label without complete
FAT tables — and the original code would have adopted that debris on the next
boot and committed an MBR entry over it, wedging the card in a state
`verifyEstablished` (the same Inspect test) can never flag. The old
always-reformat behaviour self-healed exactly that state.

So the format path now writes `gosd-data-established` into the fresh
filesystem's root after the existing `SyncDevice`, then syncs again; adoption
requires that marker. Program order plus the first barrier make the marker's
durable presence imply everything before it reached the medium. New
`diskfmt.CreateEmptyFile` / `RootFileExists` do the go-diskfs file I/O with
no mounting, reusing `openDisk` (whose lseek sizing is the 32-bit-ARM fix
from gosd-fjio — `diskfs.Open` would truncate on pi-zero-w). The marker is
NOT a dotfile: go-diskfs derives an empty 8.3 short name for a leading-dot
name and then filters that entry out of its own directory listing, so
`.gosd-data-established` would have been invisible to the check that looks
for it. That trap is documented on `CreateEmptyFile`.

Deliberately NOT part of `verifyEstablished`: once an entry is committed,
/data belongs to the app, and an app tidying away an unexpected file must
not turn its own working partition into a corruption halt. The marker is
documented as reserved in its own comment; the runtime-docs mention belongs
to gosd-zlee. `internal/image` does not write it for fixed `--data-size`
images either — those ship an MBR entry, so they take the established path
and are never adoption candidates; a fixed→expand reflash reformats exactly
as it did before the marker existed. A `MarkerExists` *error* (a GOSD-DATA
volume whose root directory won't read — the hallmark of a half-written
filesystem) reformats rather than aborting, which is what keeps an
interrupted format self-healing instead of wedging the device.

The MBR write stays last. Run's crash-safety comment now states the real
invariant: an entry always means a marker-verified filesystem, either
formatted+flushed+marked by this package or adopted only once the marker
proved an earlier boot's format finished. `verifyEstablished`'s doc comment,
which claimed "the entry is only ever written after a completed, synced
format", was corrected to match.

No other fixed-offset assumption exists in the gosd-init tree (grepped for
272, dataPartitionStartLBA, dataPartitionOffsetBytes). The remaining
occurrences are `internal/image` (the writer, i.e. the source of truth,
gosd-m70t's territory), `cmd/gosd/build_integration_test.go`'s 272MiB
assertion and `.github/workflows/ci.yml`'s `seek=557056` corruption dd —
both fixtures that assume the default boot size and will need gosd-m70t's
attention if it changes the default — and prose in docs/runtime.md
(gosd-zlee). COMPATIBILITY.md needed no change: no board × feature status
moved, and the `/data` footnote makes no reflash claim (the reflash-survival
story belongs to gosd-zlee's docs pass).

Tests (fake-driven, macOS-clean): a 1GiB boot volume puts partition 2 at
1040MiB in the committed entry and in the kernel registration; a reflashed
card whose partition holds GOSD-DATA is adopted (no format action, entry
still committed); blank/foreign/exFAT/rubble each still format; an
unreadable partition formats nothing; and an interrupted first boot (format
done, MBR write lost) resumes to byte-identical committed table without
reformatting. The default-layout tests now state 272MiB themselves rather
than sharing a production constant.

CI's `qemu first-boot data-partition expansion` job gained the real-path
proof: the build step keeps a pristine copy of the image, and a new third
boot `dd`s that copy back over the front of the card — byte-for-byte what
Imager does to an operator's card, dropping partition 2's entry and leaving
the data region untouched — then asserts `data partition re-adopted` and
`boots=3`. A reformat would have restarted hello's counter at 1, so the
counter is the data-survival assertion. The whole four-boot sequence
(create → already present → re-adopt → corrupt-halt) was run locally
against qemu-system-aarch64 — twice, once before the marker existed and
again after — and passed end to end both times; the re-adopt boot's serial
log reads "data partition re-adopted, its contents intact" / "data partition
mounted read-write at /data" / "boots=3". The marker was then confirmed on
the resulting card by hand: the data partition's root directory carries the
volume-label entry, the LFN slots for `gosd-data-established`, and its
`GOSD-D~1` 8.3 entry — written by gosd-init on a live block device on boot
1 and read back on boot 3 to authorise the adoption.
