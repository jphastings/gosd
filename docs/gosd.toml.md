# `gosd.toml`: the device's hand-editable settings

Every image `gosd build` produces carries a `gosd.toml` at the root of its
boot (FAT) partition. It is the always-present, any-flasher fallback for
configuring a device: plug the card into any computer and open `gosd.toml` in a
plain-text editor to set the hostname, join a WiFi network, change the app's
environment variables, or declare an internet tunnel. `gosd-init` reads it once,
early in boot, after mounting the boot partition.

It is written for a non-technical audience — the file leads with plain-language
instructions (which editors to use, the macOS TextEdit "Make Plain Text" trap,
what a `#` line means) — and its on-card schema is deliberately tiny and stable,
so a hand-edit can't make the device unbootable. A malformed or surprising value
is logged to the serial console and skipped, never fatal.

This page is for **app developers**: how to pre-populate and *document* that
file at build time so whoever holds the card sees sensible defaults and knows
what each setting does. For how these values behave at runtime — precedence
against a Raspberry Pi Imager wizard, what survives a reflash — see
[`docs/runtime.md`](runtime.md). For the internet-tunnel sections, see
[`docs/ingress.md`](ingress.md).

## The sections

Every section is optional; a missing file parses the same as an empty one. On a
freshly-built card each section is present as a **commented-out example** unless
you baked a value in at build time, so the user always has a template to
uncomment and edit.

| Section | Purpose | Baked in by | Depth |
|---|---|---|---|
| `hostname` | The device's network name | (the app's own name) | [`docs/runtime.md`](runtime.md) |
| `[wifi]` | WPA2-PSK or open network to join | — | [`docs/runtime.md`](runtime.md) |
| `[env]` | App environment variables | — | this page + [`docs/runtime.md`](runtime.md#app-environment-variables-gosdtoml-env) |
| `[ingress.*]` | Public internet tunnel (Cloudflare / Tailscale) | `gosd build --ingress` | [`docs/ingress.md`](ingress.md) |
| `data_flush` | Force prompt vfat writeback | `gosd build --data-flush` | [`docs/runtime.md`](runtime.md) |

Not every baked-in value gets a `gosd.toml` section, or is overridable at all:
`--support-url`, `--app-version`, and the baked app name are developer-set
report metadata (used in `LAST_FATAL_ERROR.md` crash reports, bean `gosd-my8e`),
baked into `config.json` only. There is no `gosd.toml` key and no `GOSD_*`
environment override for any of them — a user can't change or remove them by
editing the card.

## App settings — `[env]`

Your app reads its configuration from environment variables, and the `[env]`
table is where those are set on the card. Every entry is a string: a value
written bare rather than quoted boots with a "bare … not a quoted string"
warning on the serial console.

### One limitation to know

Comments live only in the `gosd.toml` a build writes. If a device is later
reflashed to a newer image and its provisioning snapshot self-heals the card's
`gosd.toml`, that re-render works from parsed *values* — TOML comments are
discarded on parse — so annotations won't reappear (the env **values** are
preserved; only the comments and commented-out lines are lost). This is cosmetic
and post-reflash only; it never changes behaviour.

## Precedence and runtime behaviour

Which source wins when a value is set in more than one place (`gosd.toml` vs a
Raspberry Pi Imager wizard's cloud-init vs the baked `config.json`), and exactly
what comes back after a reflash, is covered in
[`docs/runtime.md`](runtime.md#app-environment-variables-gosdtoml-env). The short
version for `[env]`: a hand-edited `gosd.toml` entry wins for that key; missing
or empty is always fine.
