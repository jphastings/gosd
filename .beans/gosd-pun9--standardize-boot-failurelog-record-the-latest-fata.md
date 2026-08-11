---
# gosd-pun9
title: The LAST_FATAL_ERROR.md format, and gosd-init's own fatal paths
status: todo
type: feature
priority: high
created_at: 2026-07-30T21:11:39Z
updated_at: 2026-08-11T12:04:56Z
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

- [ ] A shared renderer — `internal/faultreport` or similar — that takes a
      structured report (code, doing, problem, fix, technical detail) plus
      the device/image header and emits the epic's Markdown. Both gosd-init
      and the public `fault` package (gosd-aa1p) import it, so there is
      exactly one renderer and one format
- [ ] Golden-file tests over the rendered output, including the awkward
      cases: no fix declared (falls back to the support URL), no support URL
      baked (say so plainly rather than emitting a dangling sentence), no
      clock sync, an empty technical section, and a multi-KiB one
- [ ] Rename `boot-failure.log` → `LAST_FATAL_ERROR.md` throughout, and
      delete a stale `boot-failure.log` from the boot partition on first
      boot of an upgraded image so a card flashed by an older release
      doesn't carry two contradictory files. This is a user-facing contract
      change and needs a release note
- [ ] Call the renderer from the general `fatal()` path for every fatal that
      happens while `/boot` is mounted. **Enumerate the paths that can't be
      recorded** (early mounts, and the boot-partition mount itself — a
      failure to mount the thing you'd write to) and state in docs that
      serial is their only route
- [ ] Convert `haltForDataCorruption`'s existing hand-rolled message onto
      the renderer, keeping its recovery instructions (which are good, and
      already correctly distinguish an `expand` image from a fixed-size one)
- [ ] Assign a stable `GOSD-*` code per fatal class, and keep them listed
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
- [ ] Do NOT pair `boardDisplayName` with a board id it was not baked for:
      `sequence.go:241` lets `gosd.board=` from the hand-editable
      cmdline.txt overwrite `cfg.Board` without touching the baked display
      name, so capture the config.json board id at parse time and fall back
      to the bare id if the effective one differs (constraint documented on
      the field itself by gosd-my8e)
- [ ] Decide halt vs reboot per failure class: reboot for maybe-transient
      errors (current behaviour), halt for states no retry improves (the
      data-corruption path already halts). Record the rationale per class
- [ ] Boot counter for the `boot:` header field — decide where it lives.
      The boot partition means a write on every boot, which the risk note
      below argues against; `/data` is the likely home, with `unknown` when
      `/data` is read-only or absent
- [ ] Implement the epic's staleness rule: delete the report once the app
      has run stably, checking for the file's existence with a plain read
      first so a healthy device never remounts read-write at all
- [ ] The read-only-`/data` fallback for NON-expand images (mount failure of
      a fixed-size data partition) currently degrades silently to EROFS,
      invisible on an unattended device. Decide whether that warrants a
      report — it isn't fatal, but it means every write the app makes will
      fail, which the owner will experience as the app being broken
- [ ] Document in docs/runtime.md as a user-facing contract, replacing the
      `boot-failure.log` paragraphs at the "An established data partition is
      never repaired away" bullet
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
