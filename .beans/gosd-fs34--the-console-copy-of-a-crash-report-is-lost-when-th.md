---
# gosd-fs34
title: The console copy of a crash report is lost when the device halts
status: todo
type: bug
priority: normal
created_at: 2026-08-12T06:17:04Z
updated_at: 2026-08-12T06:17:04Z
parent: gosd-47z3
---

Found on the bench 2026-08-12, verifying `gosd-72ga`'s fix on a Pi 3B+.

`gosd-72ga` moved the console copy of a crash report from the app to
gosd-init, on the argument that gosd-init's version is strictly better —
it knows the device model, uptime and boot count the app cannot. The
UNRELEASED.md call-out states it plainly: *"gosd-init logs the complete,
real report to the serial console on its own once it commits one."*

**On the declared-fault path it doesn't.** The device halts before the
console write lands. A full 120-second capture of a real boot
(`serialcap.py`, raw termios) ends like this:

```
gosd/fault: HELLO-DEMO-FATAL — handed to gosd-init; see LAST_FATAL_ERROR.md on the boot partition; this device now stays down until someone power-cycles it
[gosd] /app (pid 156) exited with status 70 after 12.799479ms
[gosd] fatal: the app declared HELLO-DEMO-[    4.986641] reboot: System hal
```

`grep -c "error_code:"` over the whole capture finds nothing: no part of the
rendered report reached the console, and gosd-init's own final line is cut
mid-sentence by the halt.

The card copy is **complete and correct** — the write, fsync and remount all
finished. Only the console copy is lost, so this is a diagnostic-quality
issue, not a correctness or durability one.

## Why it matters anyway

It is a stated benefit of `gosd-72ga`'s chosen fix, and the reason that fix
was preferred over simply not folding the tail in. Someone with a serial
cable attached — the person most likely to be debugging — now gets *less*
than the card does, which is the opposite of what the change promised.

## Scope

Confirmed on the `fault.Fatal` (halt) path. **Not yet checked** on the two
other paths that end in a halt or a reboot: `haltForDataCorruption`, and the
generic `fatal()` reboot-after-5s path. The reboot path has a deliberate 5s
pause, so it may well be fine; the halt paths are the suspects. Check all
three before deciding the fix is complete.

## Fix direction (not locked)

Order and flushing, not content. Emit the console copy *before* initiating
the halt, and make sure the console write is flushed/synced rather than
racing `unix.Reboot`. `Rebooter.Sync()` already exists on the fatal paths for
disks — the console needs the equivalent guarantee, or simply needs to
happen earlier.

## Do this too, whichever way it goes

`docs/releases/UNRELEASED.md` currently makes the untrue claim, and it has
not shipped yet — v0.4.0 is out, this note is for the next tag. Either fix
the behaviour or soften the note before it becomes a released promise. Same
for anything in `docs/crash-reports.md` that repeats it.

## Verified working in the same run (do not re-litigate)

- `gosd-72ga`'s actual fix: the report no longer nests a copy of itself.
  1104 bytes on the Pi versus 2025 on the pre-fix NanoPi run, and the
  technical detail holds only the short pointer line.
- `device: Raspberry Pi 3 Model B Plus Rev 1.3 (pi-3b)` — the device-tree
  read on the Pi family, closing the last gap in epic `gosd-47z3`.
