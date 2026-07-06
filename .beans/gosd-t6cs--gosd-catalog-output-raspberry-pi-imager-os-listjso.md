---
# gosd-t6cs
title: 'gosd catalog output: Raspberry Pi Imager os_list.json entry'
status: completed
type: task
priority: normal
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-06T13:18:22Z
parent: gosd-b22t
---

Implements the flagship end-user flashing flow (decision 2026-07-05, see CLAUDE.md and docs/provisioning-formats.md).

`gosd build --catalog --publish-base-url=<https://...>` additionally emits an os_list.json fragment per built board image (and a combined file), with: name (app name + board), description, url (base-url + image filename), image_download_size, extract_size, extract_sha256 (all computed from the real built image), and `init_format: "cloudinit"`. Validate the shape against rpi-imager doc/json-schema/os-list-schema.json (vendor the schema into testdata, cite the pinned commit; see permalinks in docs/provisioning-formats.md).

- [x] Flag + generator + unit tests (golden JSON, schema validation)
- [x] docs/publishing.md for Go developers: host image + JSON, what URL to give end users, and the end-user steps (Imager Settings → Custom repository)
- [x] Bench re-check of device filtering in real Imager once the official-device-tags fix ships (JP's 2026-07-05 live Imager test already validated the rest of this todo: catalog loads via Custom repository, entry is selectable, customization wizard appears — see "Device-tag fix" below)

## Acceptance
A generated catalog served from a local static file server appears in Imager as a selectable OS with the customization wizard enabled (manual step, same bench session as fixture capture).

## Summary of Changes

Implemented `gosd build --catalog --publish-base-url=<url>`, which additionally
writes a Raspberry Pi Imager custom-repository `os_list.json` (a combined file
plus one per-image fragment) next to the built image(s), enabling the
flagship end-user flashing flow (WiFi/hostname customization via Imager's
Custom repository setting) instead of the "Use custom image" file picker,
which disables customization entirely (see docs/provisioning-formats.md §0).

- New `internal/catalog` package: `BuildEntry`/`WriteFiles` compute each
  entry's `extract_size`/`extract_sha256` from the real built `.img` file,
  join `--publish-base-url` with the image filename for `url` (tolerating
  any number of trailing slashes), and always set `init_format: "cloudinit"`
  (the only format gosd-init parses). `image_download_size` is kept as its
  own field, currently identical to `extract_size` since gosd distributes
  raw images today, so a future compressed-distribution mode can populate it
  differently without changing Entry's shape.
- Vendored rpi-imager's `doc/json-schema/os-list-schema.json` at the pinned
  commit `docs/provisioning-formats.md` already cites
  (`204a6eee47c2c46da453d4de4138f08619a8c0e6`, tag v2.0.10) into
  `internal/catalog/testdata/`, with a README citing the source. Rather than
  add a full JSON-Schema (draft-07) validator dependency for one flat,
  well-known object shape, `catalog_test.go` parses just the vendored
  schema's `required` field list and `init_format` enum for the "Operating
  system entry" variant and asserts generated entries satisfy them - this
  means the test still catches drift if the schema is re-pinned later,
  without a new dependency.
- Wired `--catalog`/`--publish-base-url` into `cmd/gosd build`, including an
  actionable upfront error when `--catalog` is passed without
  `--publish-base-url` (fails before any building happens).
- Board display names (e.g. "pi-zero-2w" -> "Raspberry Pi Zero 2 W") live in
  a small lookup table in `internal/catalog`, falling back to the raw board
  ID for any board not yet in that table - this naturally also covers the
  "exclude internal boards" concern from the task brief: no board-registry
  Internal marker exists yet, so entries are generated only for whatever
  boards were actually built in this invocation (never the full registry),
  which is exactly what's needed either way.
- Unit tests (`internal/catalog/catalog_test.go`): golden JSON for a fake
  build, hash/size correctness against real fixture bytes, `JoinURL` edge
  cases (trailing slashes), and the schema-required-field check above.
  Integration tests (`cmd/gosd/build_integration_test.go`) exercise
  `--catalog` end-to-end with fake artifacts, verifying the generated
  `os_list.json`/fragment against the real built image's actual sha256/size,
  plus the actionable-error case.
- `docs/publishing.md`: hosting the `.img` + `os_list.json`, exactly what
  URL end users paste into Imager's Settings -> Custom repository, and the
  resulting customization-wizard experience. Cross-linked from README's
  quickstart flashing step, which previously pointed at the "Use custom
  image" flow without noting it disables customization.

Deviations / judgment calls (schema requires more fields than the bean's own
field list mentions - `icon`, `release_date`, `devices` are all required by
os-list-schema.json's "Operating system entry" variant, confirmed against
doc/schema-notes.md's required-field list): `icon` is emitted as `""` (no
icon asset exists in this codebase yet), `release_date` uses the build's UTC
date (overridable via `catalog.Options.ReleaseDate` for reproducible/tested
output), and `devices` is set to `[boardID]` (GoSD board IDs aren't official
rpi-imager device tags, but device-based filtering appears to be an
embedded-Imager-only concept with no effect on the desktop custom-repository
flow this bean targets, and a non-empty devices array matches
doc/schema-notes.md's stated best practice over an empty one).

The manual real-Imager verification todo is left unchecked, as instructed -
it needs the bench SD-card session.

## Device-tag fix (2026-07-05, JP's live-Imager test)

JP's real-Imager test confirmed two things at once:

1. **The catalog flow itself works**: the hosted os_list.json loads via
   Settings → Custom repository, the entry appears in CHOOSE OS, and the
   full customization wizard is enabled — the core acceptance behavior of
   this bean is validated.
2. **A real bug**: the earlier "devices is set to [boardID]" judgment call
   above was wrong. Device-based filtering is NOT embedded-Imager-only:
   the desktop wizard's Device-selection page filters the OS list by
   intersecting each entry's devices array with the selected device's
   official tags, and hides entries with no overlap. With GoSD's own board
   ID as the tag, selecting "Raspberry Pi Zero 2 W" showed NO images; only
   "No filtering" revealed the entry.

Evidence for the fix (verified 2026-07-05):

- Official catalog https://downloads.raspberrypi.org/os_list_imagingutility_v4.json:
  `imager.devices` defines "Raspberry Pi Zero 2 W" with
  `"tags": ["pi3-64bit", "pi3-32bit"]` (matching_type "inclusive") — the
  Zero 2 W shares the Pi 3's tags; no Zero-2W-specific tag exists. Every
  real 64-bit OS entry uses `pi3-64bit` for Zero-2W compatibility:
  "Raspberry Pi OS (64-bit)" carries
  `["pi5-64bit", "pi4-64bit", "pi3-64bit"]`, and Zero-2W-class arm64-only
  images (Home Assistant OS 18.1 (RPi 3), OpenCCU "Pi 3, Pi Zero 2") carry
  exactly `["pi3-64bit"]`.
- Filtering source (rpi-imager v2.0.10, commit 467be3d3e88f5d83fa78c78788f6e6fdce61a47e):
  https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/src/imagewriter.cpp#L2286
  (`filterOsListWithHWTags`): an entry WITH a devices key is kept only if
  one of its strings appears in the selected device's tags; an entry
  WITHOUT the key is kept only for matching_type "inclusive" devices. The
  tags come from the catalog's own imager.devices list
  (src/hwlistmodel.cpp), not from any hardcoded table.
- Note: the commit docs/provisioning-formats.md pins (204a6eee...) no
  longer resolves on GitHub; tag v2.0.10 resolves to 467be3d3..., whose
  doc/json-schema/os-list-schema.json is byte-identical to our vendored
  copy (internal/catalog/testdata/README.md updated accordingly; the ~30
  dangling permalinks in docs/provisioning-formats.md are a candidate
  follow-up, left untouched here).

The fix (branch bean/gosd-t6cs-imager-device-tags):

- `internal/catalog`: new `boardImagerDeviceTags` lookup (mirroring the
  existing `boardDisplayNames` table — catalog concerns stay in the
  catalog package rather than widening the Board interface) maps
  `pi-zero-2w` → `["pi3-64bit"]` (64-bit only; GoSD images are arm64).
  Unavoidable consequence of the shared tag namespace: the entry also
  shows when "Raspberry Pi 3" is selected.
- `radxa-zero-3e` (and any unmapped board) falls back to its raw board ID
  as a deliberately non-matching tag. Rationale: Imager's device list
  contains only Raspberry Pi hardware, so no official tag can ever match a
  non-Pi board; the vendored schema requires `devices` but sets no
  minItems (empty would be valid), yet a non-empty self-describing tag
  follows doc/schema-notes.md's best practice and behaves identically
  (hidden under any concrete device selection, visible under "No
  filtering" — with matching_type-inclusive devices only an entry that
  OMITS the devices key entirely would leak through, which would wrongly
  show Radxa images for Pi 3/4 selections anyway). docs/publishing.md now
  states plainly that non-Pi images only appear under "No filtering"
  (an Imager limitation) and tells developers what to advise their users.
- Golden JSON, schema-validation test (now also asserts a non-empty
  devices array), and a behavioral tag-mapping test updated/added.


## Bench verification note (2026-07-06)

Verified via JP's screenshot session 2026-07-06, see
docs/images/flashing/. The seven-screenshot Raspberry Pi Imager v2.0.10
capture used for docs/flashing.md (bean gosd-ufeh) doubles as the
outstanding bench re-check for this todo: docs/images/flashing/04-choose-device.png
shows "Raspberry Pi Zero 2 W" selected on the device page, and
docs/images/flashing/05-choose-app.png (captured immediately after, same
session) shows the "hello (Raspberry Pi Zero 2 W)" entry visible and
selectable under that device selection — confirming the device-tag fix
(pi-zero-2w -> "pi3-64bit") works end-to-end in real Imager, not just by
source-code inspection. All three todos are now checked; closing.
