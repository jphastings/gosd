---
# gosd-mpr4
title: gosd push <host> CLI command
status: scrapped
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-08-21T04:40:26Z
parent: gosd-vxal
blocked_by:
    - gosd-b4ns
---

Cross-compile the app, push to the device update endpoint with the image HMAC key, drive activation, report probation outcome. Per docs/design/ab-updates.md.


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely — reflashing becomes the permanent,
only update path.** The full reasoning is on the parent epic gosd-vxal; the
short form is that reflash was already the documented baseline upgrade path,
and `--data-size=expand` re-adoption plus the `/data` config store (bean
gosd-87ip) already make a reflash preserve a device's data *and* its
operator's settings — so the gap OTA would have closed is much narrower than
when this chain was designed. What is given up, honestly: there is no way to
fix a deployed fleet without physical access to each card.

Specific to this bean: `gosd push <host>` was the developer-facing half of the
update endpoint, and with no endpoint to push to there is nothing for it to
do. The developer inner loop it was meant to shorten is served instead by
`gosd run` (qemu) off the device and by a reflash on it — and the CLI keeps
one verb fewer to support forever.
