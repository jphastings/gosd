---
# gosd-vv5o
title: rock-4se NVMe ext4 bench verification — power-cut rig
status: todo
type: task
priority: normal
created_at: 2026-08-07T14:07:40Z
updated_at: 2026-08-07T14:08:31Z
parent: gosd-lfu0
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
