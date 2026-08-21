---
# gosd-6k2n
title: GOSD-BOOT app-slot store + commit protocol
status: scrapped
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-08-21T04:40:26Z
parent: gosd-vxal
---

Per docs/design/ab-updates.md: slot.state format, write-temp/fsync/rename helpers honest about FAT atomicity, fail-safe-toward-factory parse rule for any half-committed state. Pure library + tests; no network.


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely — reflashing becomes the permanent,
only update path.** The full reasoning is on the parent epic gosd-vxal; the
short form is that reflash was already the documented baseline upgrade path,
and `--data-size=expand` re-adoption plus the `/data` config store (bean
gosd-87ip) already make a reflash preserve a device's data *and* its
operator's settings — so the gap OTA would have closed is much narrower than
when this chain was designed. What is given up, honestly: there is no way to
fix a deployed fleet without physical access to each card.

Specific to this bean: nothing else needs the slot store. It was pure library
work — `slot.state`, write-temp/fsync/rename helpers honest about FAT
atomicity, a fail-safe-toward-factory parse rule — and every one of its
consumers (gosd-1epa's probation supervisor, gosd-b4ns's endpoint) is scrapped
with it. The crash-ordering discipline it would have encoded is not lost: the
same write → sync → marker → sync pattern is already live in
`cmd/gosd-init/internal/dataexpand` and the config store, which is where the
codebase's real experience of FAT atomicity now lives.
