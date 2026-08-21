---
# gosd-1epa
title: Supervisor probation mode + three-rung rollback ladder
status: scrapped
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-08-21T04:40:26Z
parent: gosd-vxal
blocked_by:
    - gosd-6k2n
---

Extend cmd/gosd-init/internal/boot supervisor: newly-activated slot must run stably for the defined probation window before being marked good; failures fall new slot → previous good → baked factory /app. Probation must END (defined in the doc). Includes the read-write remount window for slot.state updates.


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely — reflashing becomes the permanent,
only update path.** The full reasoning is on the parent epic gosd-vxal; the
short form is that reflash was already the documented baseline upgrade path,
and `--data-size=expand` re-adoption plus the `/data` config store (bean
gosd-87ip) already make a reflash preserve a device's data *and* its
operator's settings — so the gap OTA would have closed is much narrower than
when this chain was designed. What is given up, honestly: there is no way to
fix a deployed fleet without physical access to each card.

Specific to this bean: probation mode and the three-rung rollback ladder only
exist to make an *unattended* new app version safe. With reflash as the only
update path a human is already physically present when the app changes, and
the "previous good slot" rung has no slot to fall back to. The supervisor in
`cmd/gosd-init/internal/boot` keeps its restart-with-backoff behaviour and the
crash-report path (`fault.Fatal`, `LAST_FATAL_ERROR.md`), which is the failure
story that survives: an app that cannot run says so on the card, in Markdown,
to the person holding it.
