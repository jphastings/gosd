---
# gosd-xkkm
title: 'Docs: OTA update contract'
status: scrapped
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-08-21T04:40:26Z
parent: gosd-vxal
blocked_by:
    - gosd-1epa
    - gosd-mpr4
---

Extend docs/runtime.md: what OTA can/cannot update (app only; kernel = reflash), probation semantics, failure model, LAN-trust-boundary caveat.


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely — reflashing becomes the permanent,
only update path.** The full reasoning is on the parent epic gosd-vxal; the
short form is that reflash was already the documented baseline upgrade path,
and `--data-size=expand` re-adoption plus the `/data` config store (bean
gosd-87ip) already make a reflash preserve a device's data *and* its
operator's settings — so the gap OTA would have closed is much narrower than
when this chain was designed. What is given up, honestly: there is no way to
fix a deployed fleet without physical access to each card.

Specific to this bean: there is no OTA contract left to document. The docs
work this decision *does* require was done as part of scrapping the chain
rather than here — `docs/runtime.md`'s persistence section no longer promises
that a future app-slot mechanism will spare the data partition,
COMPATIBILITY.md no longer says GoSD "will gain OTA app updates", and both
design spikes now open with the decision that stopped them. The upgrade story
users actually need is already written: `docs/design/upgrade-path.md` for the
mechanism, and the reflash/config-store sections of `docs/runtime.md` and
`docs/flashing.md` for the operator's view of it.
