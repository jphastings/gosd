---
# gosd-nplp
title: 'Cubie A5E: onboard WiFi/BT support'
status: todo
type: feature
priority: deferred
created_at: 2026-08-07T19:10:53Z
updated_at: 2026-08-07T19:10:53Z
parent: gosd-h1wv
---

JP wants the fleet featureset as complete as practical (2026-08-07), so this exists as the tracked home for cubie-a5e WiFi 6 / BT 5.4 — DEFERRED because the onboard module's driver is expected non-mainline (gosd-jpc8 found no mainline support at the fleet tag; WiFi was excluded from the epic on those grounds).

First todo when picked up: identify the actual module + chipset on the Cubie A5E (Radxa docs/schematic; likely an AIC-family or Broadcom SDIO part) and its driver status — mainline, vendor out-of-tree, or none. Decision rule follows the fleet's mainline-only policy: if the driver is not mainline (or clearly headed there), this stays deferred rather than adopting a vendor driver. If a mainline driver exists at a future fleet tag: kernel fragment + firmware via the pinned-URL manifest path (blobs are never re-hosted), wifiup integration (WPA2-PSK scope), COMPATIBILITY.md rows.
