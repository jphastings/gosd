# Vendored rpi-imager schema

`os-list-schema.json` is vendored, unmodified, from
[raspberrypi/rpi-imager](https://github.com/raspberrypi/rpi-imager) at the
pinned commit
[`467be3d3e88f5d83fa78c78788f6e6fdce61a47e`](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/doc/json-schema/os-list-schema.json)
(tag `v2.0.10`). `docs/provisioning-formats.md` originally cited this
release via commit `204a6eee...`, a SHA that stopped resolving on GitHub
(discovered 2026-07-05; the doc's links have since been repointed at
`467be3d3...`, whose schema blob was verified byte-identical to this
vendored copy). The worked
example at
[`doc/schema-notes.md`](https://github.com/raspberrypi/rpi-imager/blob/467be3d3e88f5d83fa78c78788f6e6fdce61a47e/doc/schema-notes.md)
at the same commit confirms the "Operating system entry" variant's required
field set used by `catalog_test.go`.

`catalog_test.go` reads this file directly and checks generated entries
against its `required`/`enum` arrays (see that file's `TestGeneratedEntriesSatisfySchema`
and the package doc comment on why gosd doesn't pull in a full JSON-Schema
validator dependency for this).

To re-pin at a newer rpi-imager commit, replace this file with the blob at
`doc/json-schema/os-list-schema.json` from the new commit and update the
citation above (and in `docs/provisioning-formats.md`, if that research is
being refreshed too).
