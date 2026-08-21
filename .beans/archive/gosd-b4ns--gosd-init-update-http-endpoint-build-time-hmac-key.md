---
# gosd-b4ns
title: gosd-init update HTTP endpoint + build-time HMAC key
status: scrapped
type: task
priority: deferred
created_at: 2026-07-04T21:04:04Z
updated_at: 2026-08-21T04:40:26Z
parent: gosd-vxal
blocked_by:
    - gosd-6k2n
---

The one sanctioned extra listener (CLAUDE.md): GET /update/info, PUT /update, POST /update/activate, HMAC key baked via initcfg at build time, integrity check before activation, concurrent-push rejection, app-size budget enforcement. Per docs/design/ab-updates.md.


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely — reflashing becomes the permanent,
only update path.** The full reasoning is on the parent epic gosd-vxal; the
short form is that reflash was already the documented baseline upgrade path,
and `--data-size=expand` re-adoption plus the `/data` config store (bean
gosd-87ip) already make a reflash preserve a device's data *and* its
operator's settings — so the gap OTA would have closed is much narrower than
when this chain was designed. What is given up, honestly: there is no way to
fix a deployed fleet without physical access to each card.

Specific to this bean, and the reason it is the happiest one to scrap: this
was **the one sanctioned exception to "gosd-init adds no network listeners"**.
Dropping it settles that decision instead of leaving it pending — mDNS is now
the only listener in gosd-init, full stop, and CLAUDE.md's
no-interactive-surface entry has been corrected to say so rather than promise
"and, later, the explicitly-designed update endpoint". The build-time HMAC
key, the concurrent-push rejection and the app-size budget all go with it; no
key material is baked into an image, and there is nothing on the device for a
LAN attacker to authenticate against.
