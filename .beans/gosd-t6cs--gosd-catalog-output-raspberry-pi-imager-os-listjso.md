---
# gosd-t6cs
title: 'gosd catalog output: Raspberry Pi Imager os_list.json entry'
status: todo
type: task
created_at: 2026-07-05T07:07:13Z
updated_at: 2026-07-05T07:07:13Z
parent: gosd-b22t
---

Implements the flagship end-user flashing flow (decision 2026-07-05, see CLAUDE.md and docs/provisioning-formats.md).

`gosd build --catalog --publish-base-url=<https://...>` additionally emits an os_list.json fragment per built board image (and a combined file), with: name (app name + board), description, url (base-url + image filename), image_download_size, extract_size, extract_sha256 (all computed from the real built image), and `init_format: "cloudinit"`. Validate the shape against rpi-imager doc/json-schema/os-list-schema.json (vendor the schema into testdata, cite the pinned commit; see permalinks in docs/provisioning-formats.md).

- [ ] Flag + generator + unit tests (golden JSON, schema validation)
- [ ] docs/publishing.md for Go developers: host image + JSON, what URL to give end users, and the end-user steps (Imager Settings → Custom repository)
- [ ] Manual verification with real Imager (needs SD card; coordinate with the gosd-qvoq fixture-capture bench session — same sitting)

## Acceptance
A generated catalog served from a local static file server appears in Imager as a selectable OS with the customization wizard enabled (manual step, same bench session as fixture capture).
