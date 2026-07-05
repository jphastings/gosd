---
# gosd-t6cs
title: 'gosd catalog output: Raspberry Pi Imager os_list.json entry'
status: in-progress
type: task
priority: normal
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T07:57:44Z
parent: gosd-b22t
---

Implements the flagship end-user flashing flow (decision 2026-07-05, see CLAUDE.md and docs/provisioning-formats.md).

`gosd build --catalog --publish-base-url=<https://...>` additionally emits an os_list.json fragment per built board image (and a combined file), with: name (app name + board), description, url (base-url + image filename), image_download_size, extract_size, extract_sha256 (all computed from the real built image), and `init_format: "cloudinit"`. Validate the shape against rpi-imager doc/json-schema/os-list-schema.json (vendor the schema into testdata, cite the pinned commit; see permalinks in docs/provisioning-formats.md).

- [x] Flag + generator + unit tests (golden JSON, schema validation)
- [x] docs/publishing.md for Go developers: host image + JSON, what URL to give end users, and the end-user steps (Imager Settings → Custom repository)
- [ ] Manual verification with real Imager (needs SD card; coordinate with the gosd-qvoq fixture-capture bench session — same sitting)

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
