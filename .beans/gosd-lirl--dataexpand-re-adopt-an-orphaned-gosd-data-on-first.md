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
labelled GOSD-DATA, both `FormatFAT32` and `SyncDevice` are skipped and the
run logs "data partition re-adopted, its contents intact" instead of "data
partition created, filling the card". Anything else — blank, foreign label,
exFAT, unrecognisable mid-filesystem rubble — formats exactly as before. An
Inspect *error* is neither: it aborts the boot's expansion (read-only /data,
retried next boot) rather than formatting over contents that could not be
seen. The MBR write stays last, and Run's crash-safety comment records why
adoption doesn't disturb the gosd-6sac analysis: adoption only ever removes
writes from the sequence, so the entry still commits over a complete,
durable filesystem — one made durable by an earlier boot — and a crash
before it lands leaves the same survivor for the next boot to adopt again.

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
against qemu-system-aarch64 before pushing and passed end to end; the
re-adopt boot's serial log reads "data partition re-adopted, its contents
intact" / "data partition mounted read-write at /data" / "boots=3".
