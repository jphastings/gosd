---
# gosd-vv5o
title: rock-4se NVMe ext4 bench verification — power-cut rig
status: todo
type: task
priority: normal
created_at: 2026-08-07T14:07:40Z
updated_at: 2026-08-21T06:49:52Z
---

Hardware bench follow-up to epic gosd-lfu0's code (shipped: gosd-1c0x's
disk/blockmount ext4 default + establishment/adoption state machine,
gosd-u988's golden image, gosd-ucgr's qemu-virt CI smoke). Everything below
is QEMU-verified only; this bean is the real-hardware close-out.

Needs the bench (rock-4se + NVMe SSD + sdwire rig) — not code. Not blocked
on anything: file it now, run it whenever the bench is free.

## Todos

[ ] Format: `disk.FormatAndMountWith` with zero-value `Options` (the ext4
default) against a blank rock-4se NVMe SSD — confirm the golden-image
write + EXT4_IOC_RESIZE_FS grow lands a filesystem sized to the real SSD,
not the 512MiB golden image (`internal/diskfmt/ext4golden`)
[ ] Grow-once: reboot onto the now-established volume and confirm it is
adopted (mounted, not reformatted) and NOT re-grown
[ ] Physical power-cut during sustained writes: using the sdwire bench
skill's power control, cut power to the board mid-write (a loop doing the
four-step fsync pattern from docs/runtime.md, plus a variant that skips
the fsyncs to show what the journal alone does and does not save), then
power back on and confirm: the ext4 journal replays cleanly on mount
(no fsck needed at boot), fsync'd writes survived, and un-fsync'd writes
are the ones lost — the journal-is-not-data-durability claim in
docs/runtime.md, hardware-proven rather than only qemu-asserted
[ ] Record findings (including any surprises vs the qemu behavior) in this
bean's Summary of Changes; update COMPATIBILITY.md's ext4 row/footnote
from code-complete/QEMU-tested to hardware-verified if it holds up

## Bench procedure (JP running this — 2026-08-07)

`examples/diskstorage` (from the qemu smoke, PR #194) is the ready-made test app: zero-value `disk.FormatAndMountWith` → ext4 default, durable boot counter at `<mount>/disk-boots`, HTTP readiness endpoint.

1. **Build & flash**: `gosd build --board rock-4se -o out/ ./examples/diskstorage`, flash via the sdwire rig, NVMe fitted.
2. **First boot** (serial @1.5M or `--console-baud`): expect the format→grow→marker sequence in the logs; `disk-boots` = 1; check the mounted size ≈ the NVMe's size (grow proof), not 512MiB.
3. **Adoption across clean-ish reboots**: power-cycle via the rig; counter must increment (2, 3…) — never reset (reset = reformat = adoption bug).
4. **Power-cut test**: `curl` the endpoint to confirm up, then cut power at randomized moments across several cycles (the interesting window is early boot, mid-format on a fresh card, and just-after-write). After each: counter continuity + kernel journal-replay line in dmesg output on serial. The one write per boot is brief — for sustained-write torture, loop `curl` against a small handler tweak (or just do many cycles; each boot rewrites the counter durably).
5. **Half-established debris**: once, cut power on the FIRST boot mid-format (before the marker) — next boot must cleanly reformat and start the counter at 1 (that's correct behavior, not a bug — the marker gates adoption).
6. Record boot-count reached, any resets, journal-replay observations, and timing here; COMPATIBILITY.md's ext4 row gets its hardware-verified footnote from this bean.
