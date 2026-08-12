# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.4.2 (2026-08-12).

## Breaking changes

(none yet)

## Other call-outs

- **A downloaded image's whole configuration can be injected, and survives a
  reflash (beans `gosd-dwub`, `gosd-48k0`).** `gosd build
  --config-placeholder` pads the card's `gosd.toml` out to a fixed size and
  publishes it in the `<image>.inject.json` manifest — byte ranges, hash, and
  the file's pristine text — under a new top-level `config` key. A downloader
  rewrites it exactly as it overwrites a `--placeholder` file, so a per-device
  hostname, WiFi network, `[env]` setting or [`[ingress.*]`](../ingress.md)
  tunnel credential can be spliced into an image between the CDN and the
  user's disk. What it writes is an ordinary `gosd.toml`: the device needs no
  app code for it, crash reports redact `[env]` values automatically, and the
  provisioning snapshot carries the settings across a later reflash — ingress
  sections restored as a whole unit, so a tunnel keeps working after an
  upgrade. Because the manifest publishes the pristine text, a client can
  *edit* the config it was handed rather than replace it, keeping the
  plain-language guidance gosd writes for whoever opens the card. Existing
  manifest consumers are unaffected: the key is additive and `gosd_inject`
  stays `1`. See [image
  injection](../image-injection.md#injecting-configuration-hostname-wifi-settings-ingress).
