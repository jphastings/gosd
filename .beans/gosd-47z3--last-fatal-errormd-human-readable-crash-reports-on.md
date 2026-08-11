---
# gosd-47z3
title: 'LAST_FATAL_ERROR.md: human-readable crash reports on the SD card'
status: todo
type: epic
priority: high
created_at: 2026-08-11T10:10:00Z
updated_at: 2026-08-11T12:04:44Z
---

Direction from JP (2026-08-11). GoSD devices are unattended and their owners
are not technically advanced, so a serial cable can never be the diagnosis
route. The route is: the device writes a human-readable crash report onto its
own SD card; the owner pulls the card, reads it on any computer, and either
fixes it themselves or forwards the file to the app's support site.

This epic supersedes the narrower `gosd-pun9` framing (which only covered
gosd-init's own fatal paths, and only as a `.log`). `gosd-pun9` becomes the
child that implements the format and gosd-init's own callers.

## What already exists (do not rebuild it)

`boot.Deps.WriteBootFailure` → `writeBootFailure` in
`cmd/gosd-init/internal/boot/platform_linux.go`: remount `/boot` read-write,
truncate-write `boot-failure.log`, fsync, remount read-only. Shipped with
`gosd-6sac`, documented in docs/runtime.md. It has exactly ONE caller,
`haltForDataCorruption`. The generic `fatal()` path does not use it, no
exported package reaches it, and the app's own stdout/stderr go straight to
the console fd (`sequence.go`'s `AppStarter.Start(opts.AppPath, env, console,
console)`) so gosd-init never holds a copy of a panic it could record.

## LOCKED: the file

`LAST_FATAL_ERROR.md` at the root of the boot partition — a Markdown file
with YAML frontmatter, replacing `boot-failure.log` outright. Markdown
because it renders as prose in a text editor, a Finder/Explorer preview, and
a GitHub issue, all of which a non-technical owner might use. The name is
loud and sorts near the top of a FAT root full of `kernel8.img` /
`config.txt` / `gosd.toml`.

It is overwritten, never appended: it always describes the latest fatal
issue, which is the one whoever collects the device needs.

Every producer — gosd-init's own fatals, the app-crash tail, and the public
package — formats through ONE shared writer so the file reads identically
whatever raised it.

```markdown
---
error_code: GOSD-DATA-CORRUPT
timestamp: 2026-09-11T11:57:03Z
clock: ntp-synced
uptime: 4m12s
boot: 37
device: Raspberry Pi Zero 2W (pi-zero-2w)
image: myapp 0.1.0 #a1b2c3d4
---

# myapp crash report

Your myapp device stopped while <what it was doing for its user, in human
terms>.

This file was written by the device itself, onto its own SD card, so you can
read it on any computer. Nothing was sent anywhere.

## The problem

<short human explanation of what went wrong>

## The fix

<either a concrete instruction — "set WEATHER_API_KEY in gosd.toml on this
card" — or, when the raiser declared no fix: "We don't have a specific fix
for this one. Visit <support_url> and quote the error code above.">

## What to send

If you ask anyone for help, send them **this whole file** rather than a
summary — the section below is the part they need.

## Technical detail

    <panic dump, error chain, or console tail — verbatim>
```

## LOCKED: header fields

- `error_code` — stable and greppable. gosd-init's own codes are namespaced
  `GOSD-*`; app-declared codes are whatever the app passes; the crash-tail
  path synthesises `GOSD-APP-CRASH`.
- `timestamp` / `clock` — **the clock is not trustworthy and the file must
  say so.** No board in the fleet has an RTC, and `timesync` uses
  `config.json`'s `BuildTimestamp` only as a floor to REJECT bad SNTP
  replies (`cmd/gosd-init/internal/timesync/guard.go`) — it never seeds the
  clock. Before the first successful sync the clock reads ~1970. A crash
  before networking comes up, or on a device with no network at all, is
  exactly the case a crash report exists for, so a bare wall-clock stamp
  would be confidently wrong most of the time it mattered. Emit
  `clock: ntp-synced` or `clock: unsynced — timestamp is not trustworthy`,
  and emit `timestamp: unknown` in the unsynced case rather than a 1970
  date.
- `uptime` / `boot` — always true regardless of the clock, and they answer
  the question that actually matters: did it die instantly or after four
  days? Boot count needs a durable counter (`/data`, or the boot partition
  alongside the report); decide in gosd-pun9 and note the cost — a counter
  on the boot partition means a write on every boot, which the risk note
  below argues against, so `/data` is the likely home with "unknown" when
  `/data` is read-only or absent.
- `device` — **read the hardware's own self-description from the device
  tree** (`/sys/firmware/devicetree/base/model`, equivalently
  `/proc/device-tree/model`), NOT the baked board id. Decided 2026-08-11
  after JP asked whether anything more canonical existed than the
  cmdline. `gosd.board=` is a deliberate, documented override
  (docs/runtime.md) baked into every board's cmdline template, and
  `sequence.go` lets it overwrite `cfg.Board` — but cmdline.txt is a text
  file on the FAT partition, so the least trustworthy source currently
  wins. The device tree is written by the firmware from the DTB and needs
  no new plumbing: gosd-init already mounts `/proc` and `/sys` in
  `mountEarly`, so it is readable at fatal time.

  It is also strictly more informative than our own id, because it
  distinguishes hardware GoSD deliberately conflates: `pi-3b` is ONE image
  covering the 3B and the 3B+ (the firmware picks the DTB by board
  revision), and the device tree says which one actually booted.

  Emit both, for two different readers: the model string answers "what
  hardware is this?" for the owner, and the gosd board id answers "which
  kernel and artifacts shipped?" for whoever debugs it. `boardDisplayName`
  from config.json is the fallback when the model string is missing or
  useless — qemu-virt reports `linux,dummy-virt`.
- `image` — from config.json's `appName`, `appVersion` and
  `ShortIdentity()`, all added by gosd-my8e.

## LOCKED: who writes, and when

gosd-init owns `/boot`'s mount state and is the only thing that ever writes
the report. An app never remounts `/boot` itself: it races gosd-init's own
remounts (the provisioning self-heal's `writeBootFile`, and this report),
leaves `/boot` writable under a live app — exactly the exposure the
read-only mount exists to prevent — and cannot help when the app panics
anyway. The public package hands its report to gosd-init through a file drop
in `/run` (already tmpfs, `mounts.go`), consistent with "gosd-init has no
interactive surface": no listener, no protocol.

**Write rate is bounded.** A remount-rw is the one moment a power cut can
damage the boot FAT (the risk note carried over from `gosd-pun9`, judged
acceptable because writes are tiny and rare). So: at most one report per
stable-run cycle. The first crash of a boot writes; subsequent crashes in
the same crash loop only narrate to the console; a report is written again
only after the app has run stably (`StableRunThreshold`) and then crashed
again.

**A recovered device must not look broken.** Once the app has run stably,
gosd-init deletes a stale `LAST_FATAL_ERROR.md` — but it checks for the
file's existence with a plain read first and only remounts read-write if
there is actually something to delete, so a healthy device that has never
crashed never remounts at all.

## LOCKED: secrets never reach the file (JP, 2026-08-11)

The report tells its reader to forward the whole file to a support site, so
the renderer scrubs it: every app env var VALUE becomes `{$ENV_VAR}`, and
anything an app passes to `fault.RegisterSecretString` becomes
`{secret: <label>}`. In the renderer, so every producer is covered by
construction. gosd-m6py owns it, and blocks the two beans that would
otherwise ship an unscrubbed path. The serial console stays unredacted — it
is a physically-attached channel for someone already holding the board.

## LOCKED: a declared fault halts (JP, 2026-08-11)

"This mechanism is only for fatal errors that are not considered
transient." `fault.Fatal` therefore halts rather than letting the supervisor
restart: an app calling it is asserting a restart cannot help. This governs
the DECLARED path only — an undeclared panic, whose transience is
unknowable, still restarts with backoff and merely gets a report written.

## Non-goals

- Sending anything anywhere. The report is written locally and read locally;
  no telemetry, no upload, no phone-home.
- Recording fatals that happen before `/boot` is mounted (early mounts, the
  boot-partition mount itself). Enumerate them in gosd-pun9 and accept them
  — the serial console remains their only route.
- Any interactive surface on the device.
