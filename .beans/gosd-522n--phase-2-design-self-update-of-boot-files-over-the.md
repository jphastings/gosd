---
# gosd-522n
title: 'Phase-2 design: self-update of boot files over the network (+ sneakernet bundle)'
status: scrapped
type: feature
priority: normal
created_at: 2026-07-31T09:17:50Z
updated_at: 2026-08-21T04:40:26Z
blocked_by:
    - gosd-vxal
---

Phase 2 of the upgrade-path design (bean gosd-inau, docs/design/upgrade-path.md §6). Design (not build): staged download of a boot-file payload to GOSD-BOOT, verify-then-commit, the manifest-of-owned-paths deletion scheme (§5), catalog extract_sha256 polling for update discovery, and the sneakernet bundle (route 4) as the offline carrier of the same payload format. Rides gosd-vxal's endpoint/auth (per-image HMAC). Bootloader stays pinned (locked, §0).


## Reasons for Scrapping

**JP, 2026-08-21: OTA is dropped entirely — reflashing becomes the permanent,
only update path.** This bean was blocked by gosd-vxal, and goes with it. The
full reasoning is on that epic; the short form is that reflash was already the
documented baseline upgrade path, and `--data-size=expand` re-adoption plus
the `/data` config store (bean gosd-87ip) already make a reflash preserve a
device's data *and* its operator's settings — so the gap OTA would have closed
is much narrower than when this chain was designed. What is given up,
honestly: there is no way to fix a deployed fleet without physical access to
each card.

Specific to this bean: phase 2 was self-update of the *boot files* — kernel,
initramfs, board config — and it rode gosd-vxal's endpoint and per-image HMAC
for both authentication and transport. With no endpoint there is nothing to
ride, and designing a second one purely for boot files would re-open the
listener decision this drop settles. The sneakernet bundle (route 4) folded
into phase 2's staging design and goes with it too: an offline carrier is only
worth its format and verify-then-commit machinery if something already speaks
that format, and now nothing does. Handing someone a payload file is, in the
end, no easier than handing them a flashed card.

Phase 1 — non-destructive plain reflash — is shipped and unaffected, and is
now the whole of the upgrade path rather than its first half. The manifest-of-
owned-paths deletion scheme (§5) and the catalog `extract_sha256` polling idea
stay recorded in `docs/design/upgrade-path.md`, which now marks its phase-2
section as decided-against rather than deferred.
