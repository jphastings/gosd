# Publishing: getting a GoSD image into Raspberry Pi Imager's customization wizard

GoSD's flagship end-user flashing path is a Raspberry Pi Imager
custom-repository catalog entry (locked decision — see `CLAUDE.md` and
`docs/provisioning-formats.md`). It's the only way to get Imager's
WiFi/hostname customization wizard for a GoSD image: the plain "Use custom
image" file picker disables customization entirely, regardless of the
image (see §0 of `docs/provisioning-formats.md`). Hand-editing `gosd.toml`
on the flashed boot partition is the always-present fallback for anyone not
using this flow.

The recipe: build with `--catalog`, host the resulting files, send users
the `os_list.json` URL.

## 1. Build with `--catalog`

```sh
gosd build . --catalog --publish-base-url=https://example.com/downloads
```

This builds the image(s) exactly as a normal `gosd build` would, then
additionally writes, next to each image:

- `<image>.os_list.json` — a catalog fragment containing just that image's
  entry (useful for hosting boards separately, or linking one board's
  fragment directly as another catalog's `subitems_url`).
- `os_list.json` — a combined catalog listing every image built in this
  invocation.

`--publish-base-url` is required whenever `--catalog` is given — gosd
refuses to guess where you're going to host the files, since every entry's
`url` field is `--publish-base-url` joined with the image's filename.
Omitting it fails the build immediately, before any building happens, with
an error telling you to pass one.

Every entry declares `"init_format": "cloudinit"` — the only format
gosd-init understands (see `docs/provisioning-formats.md` for why
`firstrun.sh` support is out of scope) — and its `extract_size`/
`extract_sha256` are computed from the real, uncompressed `.img` file
`gosd build` just wrote. gosd distributes raw `.img` files today, so
`image_download_size` is currently identical to `extract_size`; that's a
property of today's distribution method, not something end users or your
hosting setup need to think about.

## 2. Host the files

Upload every `.img` file and `os_list.json` to wherever `--publish-base-url`
points — any static file host works (a GitHub Release, S3/R2/GCS bucket, a
plain web server, etc.), since Imager only ever does plain HTTPS `GET`
requests for both the catalog JSON and the image. Two things matter:

- The `.img` files must be reachable at exactly `--publish-base-url` +
  filename. This is automatic: it's what `os_list.json`'s `url` fields
  already point at, as long as you upload the files gosd wrote to the
  location you passed as `--publish-base-url`.
- `os_list.json` itself needs to be reachable at some URL too — it doesn't
  have to live at `--publish-base-url`; that flag only controls the
  *image* download links. Put `os_list.json` wherever's convenient (often
  the same host, sometimes a separate one), and note that URL — it's what
  end users paste into Imager in the next step.

## 3. What end users do

1. Open Raspberry Pi Imager (desktop app), go to **Settings** (the gear
   icon in the corner, not the per-write customization gear), and find
   **Custom repository**.
2. Paste the URL of your hosted `os_list.json` (not the `.img` file) and
   save.
3. Click **CHOOSE OS** — your app now appears in the list, named and
   described the way gosd generated it (app name + board, e.g. "hello
   (Raspberry Pi Zero 2 W)"). Whether it's visible depends on the device
   selected on the wizard's device page — see "Device filtering" below
   (short version: Pi Zero 2 W images show for the "Raspberry Pi Zero 2 W"
   device; non-Pi boards need "No filtering").
4. Selecting it and continuing through the wizard shows the **full
   customization step** (hostname, WiFi, etc.) — because the catalog entry
   declares `init_format`, unlike a locally-picked `.img` file.
5. Flash as normal. Imager verifies the downloaded image against
   `extract_sha256` before writing it, refusing to write on a mismatch
   (protecting against a corrupted download or a stale cache).

Sending this URL to non-technical end users? Point them at
[`docs/flashing.md`](flashing.md) instead of this page — it walks through
the same steps above with screenshots and no jargon, and includes a
copy-paste snippet for your own README.

## Baking default app environment variables

If your app reads configuration from the environment, bake per-deployment
defaults in with repeatable `gosd build --env KEY=VALUE` flags — see
[`docs/runtime.md`'s "App environment variables"](runtime.md#app-environment-variables-gosdtoml-env)
section for the full precedence rules. Each baked default also appears
pre-filled in the card's `gosd.toml [env]` table, so whoever flashes the
card can see what you've set and override any key by editing that file —
no rebuild needed on their end.

## Keeping `/data` across upgrades (`--data-size=expand`)

If you expect to ship a second release of this app, build with
`--data-size=expand` rather than a fixed `--data-size`. A fixed size
embeds a freshly formatted `GOSD-DATA` partition inside the `.img` file
itself, so flashing any later version overwrites that region directly —
`/data` is wiped on every reflash, with no exceptions. `expand` ships no
data partition at all and has the device grow one on first boot instead;
because the image never carries those bytes, a later reflash leaves them
untouched, and the device re-adopts its existing `/data` — plus a
hand-edited hostname, WiFi network, or `gosd.toml [env]` value — instead
of starting over. See
[`docs/runtime.md`'s "Persistent storage: `/data`"
section](runtime.md#persistent-storage-data) for the full mechanics
(re-adoption, the provisioning snapshot, and what doesn't survive).

This survival only holds across a reflash that keeps the same
`--boot-size` the earlier release used — see below.

## Sizing the boot volume (`--boot-size`)

`gosd build`'s `GOSD-BOOT` partition defaults to 256MiB, enough for every
stock board's kernel and initramfs. An app bundling a large companion
binary (see [`--with-external`](runtime.md#bundling-a-companion-binary---with-external))
may need more:

```sh
gosd build . --board pi-zero-2w --boot-size 1GiB
```

The build fails with an actionable error naming `--boot-size` if your
payload still doesn't fit, and every successful build prints a one-line
usage report (`<board> boot volume: <used> / <size> used (<pct>%)`) so you
can watch your headroom shrink across releases before it becomes a
problem.

**The size you ship becomes part of your app's on-disk layout — its
layout ABI.** It fixes where the data partition starts on the card. A
later release that changes `--boot-size`, in either direction, moves that
offset: the device can no longer recognize what's already on the card as
its own `/data`, and the next reflash formats a fresh one instead of
adopting it — a clean wipe, not corruption, but real data loss for anyone
who upgrades. If a release must change `--boot-size`, say so at
release-notes level, the same as any other breaking change.

## Device filtering: which boards show up for which device selection

Imager's first wizard page asks the user to pick their device, and then
**hides every OS entry whose `devices` array shares no tag with that
device's official tag list** (only "No filtering" shows everything).
The tags are Imager's own vocabulary — defined in the official catalog's
device list, covering Raspberry Pi models only — so gosd fills each
entry's `devices` with the matching official tags where they exist:

- **`pi-zero-2w`** entries carry `pi3-64bit`: Imager defines the
  "Raspberry Pi Zero 2 W" device with the Pi 3's tags
  (`pi3-64bit`/`pi3-32bit` — there is no Zero-2W-specific tag), and GoSD
  images are 64-bit only. Users who select **Raspberry Pi Zero 2 W** (or
  Raspberry Pi 3, an unavoidable consequence of the shared tags) will see
  your image.
- **`radxa-zero-3e`** (and any other non-Raspberry-Pi board) has no
  official tag that can ever match — Imager's device list contains only
  Raspberry Pi hardware. Those entries keep the gosd board ID as a
  deliberately non-matching tag, which means they **only appear when the
  user selects "No filtering"** on the device page. This is a limitation
  of Raspberry Pi Imager itself, not something a catalog can work around;
  tell your non-Pi users to pick "No filtering" (the `gosd.toml`
  hand-edit fallback also always works, with any flasher).

## Combining catalogs

If you publish more than one GoSD app, or want your catalog listed
alongside other operating systems, `os_list.json`'s only required top-level
key is `os_list` — see `doc/schema-notes.md` at the pinned rpi-imager
commit cited in `docs/provisioning-formats.md` for the full shape, including
`subitems_url` for linking multiple catalogs together. gosd's generated
files intentionally omit the optional top-level `imager` key (that's
metadata for the Imager application itself, not for an individual OS
entry), so they compose cleanly as a `subitems_url` target from a
hand-written parent catalog.

## Verifying locally before publishing

Point Imager's custom-repository setting at a local static file server
(e.g. `python3 -m http.server` in the output directory) to see exactly what
end users will see before uploading anywhere. This is the same manual
check tracked as an open bench-verification todo on bean `gosd-t6cs`.
