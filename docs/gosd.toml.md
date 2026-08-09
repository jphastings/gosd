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
| `hostname` | The device's network name | `gosd build --hostname` | [`docs/runtime.md`](runtime.md) |
| `[wifi]` | WPA2-PSK or open network to join | `gosd build --wifi-ssid/--wifi-pass` | [`docs/runtime.md`](runtime.md) |
| `[env]` | App environment variables | `gosd build --env` / `--env-file` | this page + [`docs/runtime.md`](runtime.md#app-environment-variables-gosdtoml-env) |
| `[ingress.*]` | Public internet tunnel (Cloudflare / Tailscale) | `gosd build --ingress` | [`docs/ingress.md`](ingress.md) |
| `data_flush` | Force prompt vfat writeback | `gosd build --data-flush` | [`docs/runtime.md`](runtime.md) |

## Documenting your app's settings — `[env]`

Your app reads its configuration from environment variables. The `[env]` table
is where those are set on-device, and where you, the developer, decide what the
card ships with and how it's explained.

### Baking default values — `--env`

`gosd build --env KEY=VALUE` (repeatable) bakes a default. It lands in two
places: `config.json` inside the image (the developer default that applies even
if the user deletes `gosd.toml` entirely) and, pre-filled, in the card's
`gosd.toml [env]` section so the user can see and override it:

```console
$ gosd build . --env API_URL=https://example.com --env LOG_LEVEL=info
```

```toml
[env]
API_URL = "https://example.com"
LOG_LEVEL = "info"
```

That is enough when the defaults are self-explanatory. When they aren't — or
when you want to *offer* a setting without turning it on — write the section
yourself with `--env-file`.

### Documenting and suggesting settings — `--env-file`

`gosd build --env-file <path>` points at a TOML file whose contents become the
card's `[env]` section **verbatim**. You write the section body — the `KEY =
"value"` lines and comments — exactly as you want it to appear, and gosd splices
it in under the `[env]` header. This is the way to ship per-key comments and
*suggested* (commented-out) settings a user opts into.

The file is only the **body** of `[env]`: no `[env]` header of its own (gosd adds
it), and no other TOML section. It is a normal, lintable TOML file on its own —
comments and top-level `KEY = value` pairs:

```toml
# app-env.toml
# uncomment this if you want the demo to run
# RUN_DEMO = true

# Where telemetry is posted; leave blank to disable
API_URL = "https://example.com"
```

Building with `--env-file app-env.toml` produces exactly this `[env]` section on
the card:

```toml
[env]
# uncomment this if you want the demo to run
# RUN_DEMO = true

# Where telemetry is posted; leave blank to disable
API_URL = "https://example.com"
```

What each part means:

- A **comment** (`# …`) is yours — write as many lines as you like, wherever you
  like. gosd never adds its own preamble to a section you author.
- A **suggested** setting is just a commented-out assignment (`# RUN_DEMO = …`).
  It is off until the user removes the `#`. Because it's commented out it is
  **not** baked into `config.json` either — it does nothing until uncommented, at
  which point `gosd-init` applies it like any other hand-edit.
- An **active** setting (an uncommented `KEY = "value"`) *is* baked into
  `config.json` as a default, so it applies even if the user later deletes
  `gosd.toml`.

gosd validates the file at build time so a mistake fails the build rather than
shipping a `gosd.toml` the device can't parse: it must be valid TOML with **no
section headers** (its own `[env]`, a stray `[wifi]`, `[[anything]]` — all
rejected), and every active key must follow the same rules as `--env`
(`[A-Za-z_][A-Za-z0-9_]*`, no reserved `GOSD_*`).

`--env-file` and `--env` can't be combined — the file *is* the whole `[env]`
section. Use one or the other.

### A note on quoting

Because the section is spliced verbatim, you control exactly how each line reads —
quoted or not. But an **active** (uncommented) bare scalar like `PORT = 8080`
boots with a "bare … not a quoted string" warning on the serial console (every
`[env]` value your app receives is a string), so quote values you mean to keep:
write `PORT = "8080"`. The build prints the same warning so you catch it early. A
**commented-out** suggestion is free text until the user uncomments it, so how you
write it is entirely up to you (`# RUN_DEMO = true` is fine).

### One limitation to know

Your comments live only in the `gosd.toml` this build writes. If a device is later
reflashed to a newer image and its provisioning snapshot self-heals the card's
`gosd.toml`, that re-render works from parsed *values* — TOML comments are
discarded on parse — so your annotations won't reappear (the env **values** are
preserved; only the comments and commented-out lines are lost). This is cosmetic
and post-reflash only; it never changes behaviour.

## Precedence and runtime behaviour

Which source wins when a value is set in more than one place (`gosd.toml` vs a
Raspberry Pi Imager wizard's cloud-init vs the baked `config.json`), and exactly
what comes back after a reflash, is covered in
[`docs/runtime.md`](runtime.md#app-environment-variables-gosdtoml-env). The short
version for `[env]`: a hand-edited `gosd.toml` entry overrides the baked default
for that key; missing or empty is always fine.
