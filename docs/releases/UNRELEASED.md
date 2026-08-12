# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.4.2 (2026-08-12).

## Breaking changes

(none yet)

## Other call-outs

- **Environment variables can be injected into a downloaded image, and
  survive a reflash (bean `gosd-dwub`).** `gosd build --env-placeholder
  <size>` reserves space for the body of the card's `[env]` table and
  publishes that region's byte ranges in the `<image>.inject.json` manifest,
  under a new top-level `env` key. A downloader overwrites it exactly as it
  overwrites a `--placeholder` file, and what it writes arrives as an
  ordinary `gosd.toml` setting: the app needs no code for it, crash reports
  redact it automatically, and — because it is the operator's own value as
  far as the device is concerned — the provisioning snapshot carries it
  across a later reflash, which an injected file of gosd's own could never
  do. Existing manifest consumers are unaffected: the key is additive,
  `gosd_inject` stays `1`, and a client that doesn't know it ignores it.
  See [image injection](../image-injection.md#injecting-environment-variables).
