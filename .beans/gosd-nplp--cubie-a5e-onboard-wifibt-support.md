---
# gosd-nplp
title: 'Cubie A5E: onboard WiFi/BT support'
status: scrapped
type: feature
priority: deferred
created_at: 2026-08-07T19:10:53Z
updated_at: 2026-08-21T04:43:59Z
parent: gosd-h1wv
---

JP wants the fleet featureset as complete as practical (2026-08-07), so this exists as the tracked home for cubie-a5e WiFi 6 / BT 5.4 — DEFERRED because the onboard module's driver is expected non-mainline (gosd-jpc8 found no mainline support at the fleet tag; WiFi was excluded from the epic on those grounds).

First todo when picked up: identify the actual module + chipset on the Cubie A5E (Radxa docs/schematic; likely an AIC-family or Broadcom SDIO part) and its driver status — mainline, vendor out-of-tree, or none. Decision rule follows the fleet's mainline-only policy: if the driver is not mainline (or clearly headed there), this stays deferred rather than adopting a vendor driver. If a mainline driver exists at a future fleet tag: kernel fragment + firmware via the pinned-URL manifest path (blobs are never re-hosted), wifiup integration (WPA2-PSK scope), COMPATIBILITY.md rows.


## Reasons for Scrapping

**JP, 2026-08-21: superseded by gosd-woox, the single upstream watch list.**
Not abandoned, and emphatically not a change of intent — JP still wants the
fleet featureset as complete as practical (2026-08-07), which is why this was
tracked rather than dropped in the first place. It is re-homed because its
unlock is the same upstream cadence gosd-36yy and gosd-vo75 were waiting on,
and one checklist beats three reminders.

gosd-woox's item W3 carries this bean whole: that the first step when picked
up is **identifying the actual module and chipset** on the Cubie A5E (Radxa
docs or schematic; likely an AIC-family or Broadcom SDIO part) and its driver
status, since that has never been done; and the decision rule that makes this
deferred rather than open — **the fleet's mainline-only policy: if the driver
is not mainline, or clearly headed there, it stays deferred rather than
adopting a vendor driver.** If a mainline driver does exist at a future fleet
tag: kernel fragment, firmware via the pinned-URL manifest path (blobs are
never re-hosted), `wifiup` integration within WPA2-PSK scope, COMPATIBILITY.md
rows.
