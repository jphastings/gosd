---
# gosd-pun9
title: The LAST_FATAL_ERROR.md format, and gosd-init's own fatal paths
status: in-progress
type: feature
priority: high
created_at: 2026-07-30T21:11:39Z
updated_at: 2026-08-11T13:53:13Z
parent: gosd-47z3
blocked_by:
    - gosd-my8e
---

Part of epic gosd-47z3, which supersedes this bean's original narrower
framing (`boot-failure.log`, gosd-init's own paths only). Read the epic
first: it carries the LOCKED file format, header fields, write-rate rule and
staleness rule. This bean implements that format and converts gosd-init's own
fatal paths onto it. It is the first consumer, so the shared renderer lands
here.

Original direction from JP (2026-07-30, during gosd-6sac review): GoSD
devices are unattended, so the latest run's fatal issue must be discernable
without a serial cable — by pulling the card and reading it on any computer.
Extended by JP 2026-08-11: the file is `LAST_FATAL_ERROR.md`, Markdown, and
every producer formats through the same renderer so it reads identically
whatever raised it.

## Where it stands

The seed shipped with gosd-6sac: `boot.Deps.WriteBootFailure` →
`writeBootFailure` in `cmd/gosd-init/internal/boot/platform_linux.go`
(remount `/boot` read-write, overwrite the file, sync, remount read-only).
Its one caller is `haltForDataCorruption`. The remount/sync/restore mechanism
is proven and stays; what changes is the filename, the content, and how many
paths reach it.

## Todos

- [x] A shared renderer — `internal/faultreport` or similar — that takes a
      structured report (code, doing, problem, fix, technical detail) plus
      the device/image header and emits the epic's Markdown. Both gosd-init
      and the public `fault` package (gosd-aa1p) import it, so there is
      exactly one renderer and one format
- [x] Golden-file tests over the rendered output, including the awkward
      cases: no fix declared (falls back to the support URL), no support URL
      baked (say so plainly rather than emitting a dangling sentence), no
      clock sync, an empty technical section, and a multi-KiB one
- [x] Rename `boot-failure.log` → `LAST_FATAL_ERROR.md` throughout, and
      delete a stale `boot-failure.log` from the boot partition on first
      boot of an upgraded image so a card flashed by an older release
      doesn't carry two contradictory files. This is a user-facing contract
      change and needs a release note
- [x] Call the renderer from the general `fatal()` path for every fatal that
      happens while `/boot` is mounted. **Enumerate the paths that can't be
      recorded** (early mounts, and the boot-partition mount itself — a
      failure to mount the thing you'd write to) and state in docs that
      serial is their only route
- [x] Convert `haltForDataCorruption`'s existing hand-rolled message onto
      the renderer, keeping its recovery instructions (which are good, and
      already correctly distinguish an `expand` image from a fixed-size one)
- [x] Assign a stable `GOSD-*` code per fatal class, and keep them listed
      somewhere a support page could mirror
- [ ] Render `device:` from the DEVICE TREE, not the board id — see the
      epic's locked header-fields section for the full argument. Read
      `/sys/firmware/devicetree/base/model` (NUL-terminated, needs
      trimming), behind the usual interface seam with a fake-driven test
      that passes on macOS and a `platform_linux.go` real read. Fall back
      to config.json's `boardDisplayName` when it is unreadable, and emit
      the gosd board id alongside either way. **Verify on the bench that
      the file exists and reads sensibly on at least one Pi and one
      Rockchip board** — no code in this repo reads the device tree today,
      so its availability under our trimmed kernels is assumed, not proven.
      Note what qemu-virt reports (expected: `linux,dummy-virt`) and make
      sure the fallback handles it gracefully rather than printing it
- [x] Do NOT pair `boardDisplayName` with a board id it was not baked for:
      `sequence.go:241` lets `gosd.board=` from the hand-editable
      cmdline.txt overwrite `cfg.Board` without touching the baked display
      name, so capture the config.json board id at parse time and fall back
      to the bare id if the effective one differs (constraint documented on
      the field itself by gosd-my8e)
- [x] Decide halt vs reboot per failure class: reboot for maybe-transient
      errors (current behaviour), halt for states no retry improves (the
      data-corruption path already halts). Record the rationale per class
- [x] Boot counter for the `boot:` header field — decide where it lives.
      The boot partition means a write on every boot, which the risk note
      below argues against; `/data` is the likely home, with `unknown` when
      `/data` is read-only or absent
- [x] Implement the epic's staleness rule: delete the report once the app
      has run stably, checking for the file's existence with a plain read
      first so a healthy device never remounts read-write at all
- [x] The read-only-`/data` fallback for NON-expand images (mount failure of
      a fixed-size data partition) currently degrades silently to EROFS,
      invisible on an unattended device. Decide whether that warrants a
      report — it isn't fatal, but it means every write the app makes will
      fail, which the owner will experience as the app being broken
- [x] Update docs/runtime.md, replacing the `boot-failure.log` paragraphs
      at the "An established data partition is never repaired away" bullet
- [ ] The developer-facing guide already exists as docs/crash-reports.md
      (written 2026-08-11 ahead of the code, carrying a "partly built"
      status banner in the ab-updates.md house style). Keep it TRUE as each
      slice lands, and delete the banner only when the whole epic is done —
      it is the file JP points other agents at, so a stale claim there
      misleads them directly
- [ ] Link the guide from README.mds docs list once the feature actually
      works. It is deliberately unlinked while unbuilt, so nobody finds it
      by browsing and believes the API is importable
- [ ] Verify on the bench: pull the card after a forced fatal and confirm
      the file is present, complete and renders as prose on macOS

## Crash-ordering argument (required before review)

The write is remount-rw → truncate → write → fsync → remount-ro, and every
current caller halts immediately after. A power cut inside that window can
damage the boot FAT — accepted at gosd-6sac review because writes are tiny
and rare. That argument only holds while writes STAY rare, which is what the
epic's one-report-per-stable-run-cycle rule and the read-before-remount
staleness check exist to guarantee. Anything that makes this path fire more
often reopens the question.

Note the asymmetry with the rest of the codebase: this is the one on-disk
write with no write → sync → marker → sync commit record, because a
half-written crash report is not a state anything later adopts — it is read
by a human who can see it is truncated. Say so explicitly in the review
rather than letting it read as an oversight.

## Summary of Changes

The renderer, the file, and gosd-init's own fatal paths are done; the two
bench-verification todos and the two docs/README todos that depend on the rest
of the epic are deliberately still open (see "What is NOT done" below).

**`internal/faultreport` — the one renderer.** `Render(Report, Context)
Result` takes the human content (`Code`, `Doing`, `Problem`, `Fix`, `Detail` —
the same field names gosd-aa1p's public `fault.Report` sketches) plus a
`Context` of everything the raiser can't know (app/board/image identity,
clock, uptime, boot count, device-tree model, support URL, and the redaction
rules), and emits the epic's Markdown. `FileName`/`LegacyFileName` are
exported so nothing spells the filenames itself. It lives in `internal/` so
both gosd-init and the future public `fault` package import it and exactly one
renderer exists.

Two things the renderer owns so no producer can get them wrong:

- **Redaction is applied to the body by construction** (`Context.Secrets`,
  `Result.SkippedSecrets`). gosd-m6py's remaining job is only to *discover*
  the rules (the env scan and the `/run` registration channel) and hand them
  over; the seam it needs already exists and is tested, so its wiring PR
  shouldn't have to change this API. The frontmatter is generated from
  known-safe fields and is deliberately not scrubbed.
- **`BoardDisplayName` is only paired with the board id it was baked for**
  (`Context.BoardDisplayNameFor`), so gosd-my8e's documented `gosd.board=`
  trap is structurally impossible rather than a rule each caller has to
  remember. `sequence.go` captures `cfg.Board` before the cmdline override,
  and a mismatch renders the bare effective id.

**Golden tests** (`internal/faultreport/testdata`, `-update` regenerates)
cover the awkward cases this bean lists: no fix with a support URL, no fix
without one, an unsynced clock on an image with nothing baked, an empty
technical section, a multi-KiB goroutine dump, an unusable device-tree model,
a cmdline-overridden board, and secrets landing in prose as well as in the
detail.

**gosd-init.** `fatal()` now takes a `fatalClass` (stable `GOSD-*` code,
gerund action, owner-facing prose, halt-vs-reboot) and records through the
renderer whenever the boot partition is mounted; `haltForDataCorruption` is
one such class, keeping its expand-vs-fixed-size recovery instructions as the
report's `Fix`. `boot.fatalReporter` holds the header and enforces the two
write-rate rules; `Supervisor` gained `OnStableRun` and an injectable `After`
so "the app has recovered" is announced by a timer *while the app is still
running* — which matters because the healthy device is the one whose app
never exits at all, and a stale report on it would otherwise never be
deleted. `boot.FaultReportDeps` bundles the platform seams
(write/exists/remove/device-model/uptime/clock-synced/count-boot), all
nil-checked.

### Decisions this bean owed

- **Halt vs reboot, per class.** `GOSD-DATA-CORRUPT` halts — no retry improves
  a corrupt filesystem, and a reboot loop grinds the card and buries the
  report. `GOSD-BOOT-MOUNT` and `GOSD-EARLY-MOUNT` reboot: both have plausible
  transient causes (a slow SD controller, a device still probing), and a
  device that fixes itself beats one waiting for a visit. Recorded in the
  `fatalClass` table and in the docs table a support page can mirror.
- **The boot counter lives on `/data`** (`/data/.gosd-boot-count`, written
  through `provsnapshot.WriteFileDurably`), exactly as the epic guessed: the
  boot partition would mean a remount-rw on every single boot, which is the
  write-rate rule's whole objection. A read-only or absent `/data` reports
  `unknown`, as does an unparseable counter (which is replaced rather than
  wedging every future boot). The data-corruption halt therefore always
  reports `boot: unknown` — it happens before the data mount, and `/data` is
  what is broken.
- **Which fatals can never be recorded:** `GOSD-EARLY-MOUNT` and
  `GOSD-BOOT-MOUNT`, both of which precede (or are) the mount of the partition
  the report would be written to. The console line says so explicitly rather
  than leaving silence, and the docs table names them.
- **The read-only-`/data` fallback gets no report** — and the reason is
  concrete rather than a judgement call: gosd-init *cannot tell* a
  `--data-size=0` image (no partition by design) from a fixed-size image whose
  partition has vanished, because config.json bakes `dataExpand` and
  `dataFilesystem` but no data size. Reporting on what it can see today would
  fire on every legitimately partition-less image. It is also not fatal, and
  LAST_FATAL_ERROR.md is defined as the latest *fatal* issue, so a merely
  degraded device would start looking crashed. Making this reportable needs a
  new config.json field ("this image expects a data partition"), which is a
  separate bean's worth of on-card contract.

### Deviation from the epic's locked example, flagged rather than silent

The epic (and the crash-report guide) show `image: myapp 0.1.0 #a1b2c3d4`
unquoted. That is not what it appears to be: in YAML, a space followed by `#`
begins a comment, so a parser reads that field as `myapp 0.1.0` and silently
drops the build identity — the one field whoever debugs the report most needs.
The renderer therefore quotes a header value whenever emitting it bare would
change what a parser reads back, so the line renders as
`image: "myapp 0.1.0 #a1b2c3d4"`. Same characters, two more quote marks, and
the "machine-readable header" claim stays true. If JP would rather have the
example's exact bytes, the fix is one predicate in `yamlScalar`.

### Adversarial pass

- **The write-rate rule has a worst case worth knowing before gosd-s9uq wires
  the app-crash path.** An app that reliably dies *just after*
  `StableRunThreshold` produces two boot-FAT remounts per cycle — one to
  delete the now-stale report, one to write the new one — roughly 200/hour on
  a 35-second cycle. Nothing gosd-init raises today can reach that (every
  fatal reboots or halts), so this is a note for the crash-tail bean rather
  than a defect here: it may want to bound re-arming per boot. The two locked
  rules can't both hold otherwise — deleting is what stops a recovered device
  looking broken.
- **Deleting on recovery destroys evidence**, by design: an owner who
  power-cycles "to see if it fixes itself" and then pulls the card finds
  nothing, if the app then ran 30 seconds. That is the epic's locked trade and
  the docs say it plainly, but it is the behaviour most likely to surprise.
- **A stale report is deleted only after a plain read**, so the healthy device
  — the overwhelming majority of boots — never remounts the boot partition at
  all. Pinned by a test asserting the *absence* of a remount, not just its
  presence when there is something to delete.
- **A failed write leaves the gate armed.** An earlier version closed it on
  every attempt, which would have let one transient write failure suppress
  every later report for the rest of the run cycle.
- `record` and `markStableRun` share a mutex because the stable-run timer runs
  on its own goroutine; that goroutine is `PanicGuard`-wrapped, since a panic
  in PID 1 is a dead appliance.

### Crash-ordering argument, as shipped

Unchanged in mechanism from gosd-6sac, and still the deliberate exception to
this codebase's write → sync → marker → sync rule: remount-rw → truncate →
write → fsync → remount-ro, with no commit record. That is not an oversight.
A commit record exists so a later boot can prove a write finished before
adopting it as state — and *nothing ever adopts this file*. Its only consumer
is a human, who can see for themselves that a report stops mid-sentence, and
a marker would mean a second write inside the very window the design is trying
to keep short. What a power cut can damage here is the boot FAT itself, and
that risk was accepted at gosd-6sac review because writes are tiny and rare —
which is precisely why the one-report-per-stable-run-cycle rule and the
read-before-remount staleness check are enforced inside `fatalReporter` rather
than left to each caller. The deletion path (`removeBootFiles`) syncs the
filesystem before restoring the read-only mount and reports a failed restore
rather than swallowing it, because unlike the report write it runs on a device
that carries on booting with a live app.

### Evidence beyond the unit tests

CI's `qemu first-boot data-partition expansion` job already forces the
data-corruption halt on a real (virtual) card, and now asserts the console's
"recorded this failure to LAST_FATAL_ERROR.md" line — which only prints once
the write returned. So the remount-rw → write → fsync → remount-ro path is
proven against a live vfat boot partition, not only against fakes. What that
job still can't show is the file read back from the card: CI has no
FAT-inspection tooling, and adding some was out of scope here. That is what
the remaining bench todo covers.

While wiring that assertion, the data-corruption class stopped wrapping its
own cause: dataexpand's error already reads as a whole sentence, so
`fatal()` was saying it twice ("reading the data partition failed: the data
partition is corrupt: ..."). A `fatalClass` may now omit its action, which
also restores the exact console line that job has always grepped for.

### What is NOT done, and why

- **Bench verification of the device-tree read is outstanding** (a Pi and a
  Rockchip board), so nothing here claims it verified: no code in this repo
  has ever read `/sys/firmware/devicetree/base/model`, and its availability
  under our trimmed kernels remains assumed. The fallback chain is built and
  tested to make that safe — an unreadable, empty, or compatible-string-shaped
  model (qemu-virt's `linux,dummy-virt`) falls back to the baked board display
  name, then to the bare board id, then to `unknown` — and the crash-report
  guide's status banner records that the read is unconfirmed on hardware.
- **Pulling a card after a forced fatal is likewise still a bench todo.**
- **README stays unlinked to the guide**, per the todo above: the `fault` API
  is still not importable, so browsing to it would mislead.
- **The guide's status banner stays**, rewritten for what now exists; the todo
  says to remove it only when the whole epic lands, and two of its sections
  now carry their own "not built yet" notes.
- **COMPATIBILITY.md is deliberately untouched**: crash reports are
  board-independent, and the only per-board variance (whether the device tree
  names the hardware) is cosmetic with a guaranteed fallback, so a matrix row
  would be a placeholder for something no board fails at. Worth adding if the
  bench pass finds a board with no device tree at all.
