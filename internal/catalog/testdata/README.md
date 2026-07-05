# Vendored rpi-imager schema

`os-list-schema.json` is vendored, unmodified, from
[raspberrypi/rpi-imager](https://github.com/raspberrypi/rpi-imager) at the
pinned commit
[`204a6eee47c2c46da453d4de4138f08619a8c0e6`](https://github.com/raspberrypi/rpi-imager/blob/204a6eee47c2c46da453d4de4138f08619a8c0e6/doc/json-schema/os-list-schema.json)
(tag `v2.0.10`), the same commit `docs/provisioning-formats.md` cites
throughout. The worked example at
[`doc/schema-notes.md`](https://github.com/raspberrypi/rpi-imager/blob/204a6eee47c2c46da453d4de4138f08619a8c0e6/doc/schema-notes.md)
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
