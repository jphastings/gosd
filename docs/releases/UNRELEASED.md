# Unreleased

Release-notes-level call-outs — breaking changes above all — accumulate here
between CLI `vX.Y.Z` tags. At each release they fold into the tag's notes
(`gh release create --notes-file`, edited as needed) and this file resets to
this stub. Last folded into: v0.4.2 (2026-08-12).

## Breaking changes

(none yet)

## Other call-outs

- **Every image now carries its settings as a directory of files on the boot
  partition, and each of them can be injected into a downloaded image (epic
  `gosd-rw6n`).** `config/` holds one setting per file — `hostname`,
  `wifi/ssid`, `env/<NAME>`, an ingress agent's token — each documented by a
  `<name>.explain.md` sidecar the card's owner can read, and each padded to a
  reservation whose byte ranges the `<image>.inject.json` manifest publishes
  under a new top-level `config` array. An app supplies its own settings, and
  overrides gosd's, with `gosd build --config-dir <dir>` (defaulting to a
  `config/` directory beside its main package). **`--env`, `--env-file`,
  `--hostname`, `--wifi-ssid` and `--wifi-pass` are removed**: the overlay
  directory is the developer's input, and the Imager wizard is the
  customer's. See [image injection](../image-injection.md#injecting-settings).
