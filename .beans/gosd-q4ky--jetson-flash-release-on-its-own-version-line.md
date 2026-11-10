---
# gosd-q4ky
title: 'jetson-flash: release on its own version line'
status: completed
type: task
created_at: 2026-11-10T11:00:14Z
updated_at: 2026-11-10T11:00:14Z
---

`jetson-flash` gets its own releases, tagged `jetson-flash/vX.Y.Z` and cut by knope like every other package, rather than riding along on `gosd/v*` (JP, 2026-11-10). It changes on its own schedule and has different users from the CLI.

The first release can be any version under 1.0.0 (JP), so it's whatever a normal change file gives: `v0.0.1`.

## Todo

- [x] Add a `jetson-flash` package to `knope.toml`, with its version and changelog files.
- [x] Trigger the binary workflow on `jetson-flash/v*` instead of `gosd/v*`.
- [x] Point the flashing guide's downloads at `jetson-flash/v*` releases.
- [x] Add it to the release documentation.

## Summary of Changes

`jetson-flash` is now a knope package with its own version file (`build/jetson-flash/VERSION`) and changelog (`docs/releases/jetson-flash.md`). `release-jetson-flash.yml` fires on `jetson-flash/v*` tags, and the flashing guide and release documentation point at that line. The change file gives the first release.
