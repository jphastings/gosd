---
# gosd-4hz1
title: Imager wizard hostname is shadowed by the baked gosd.toml hostname on every image
status: todo
type: bug
priority: high
created_at: 2026-07-31T20:14:57Z
updated_at: 2026-07-31T20:14:57Z
---

Found during gosd-ry3b's implementation (PR #161), recorded in that bean;
filed separately here.

`gosd build` always renders `hostname = "..."` into the card's gosd.toml
(the --hostname flag defaults to the sanitized main-package name), and
the locked provisioning precedence is gosd.toml > cloud-init > baked
config.json. Consequence: the hostname an end user types into Imager's
customization wizard (cloud-init user-data) is ALWAYS outranked by the
baked default — the wizard's headline feature silently doesn't take
effect. Wizard WiFi is unaffected (gosd.toml's [wifi] block ships
commented out unless baked).

**Fix direction (decide in this bean):** either stop rendering an
uncommented hostname line into gosd.toml at build time (ship it
commented, like [wifi], so the precedence chain falls through to
cloud-init), or make the renderer distinguish "baked default" from
"operator-set" hostname. The first is simpler and matches the [wifi]
pattern; check interaction with gosd-ry3b's snapshot classification
(a freshly flashed card's gosd.toml equalling the rendered template is
how hand-edits are detected — shipping the hostname commented actually
sharpens that test). Behavioral test: wizard-provisioned hostname takes
effect on a stock image; hand-edited gosd.toml hostname still wins.
