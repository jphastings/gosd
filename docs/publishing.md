# Publishing: getting a GoSD image into Raspberry Pi Imager's customization wizard

This is the flagship end-user flashing path (locked decision, see
`CLAUDE.md` and `docs/provisioning-formats.md`): a Raspberry Pi Imager
custom-repository catalog entry. It's the only supported way to get
Imager's WiFi/hostname customization wizard to appear for a GoSD image —
Imager's plain "Use custom image" file picker disables customization
entirely, regardless of the image (see §0 of
`docs/provisioning-formats.md`). Hand-editing `gosd.toml` on the flashed
boot partition remains the always-present fallback for anyone not using
this flow.

## 1. Build with `--catalog`

```sh
gosd build . --catalog --publish-base-url=https://example.com/downloads
```

This builds the image(s) exactly as a normal `gosd build` would, then
additionally writes, next to each image:

- `<image>.os_list.json` — a catalog fragment containing just that image's
  entry (useful if you want to host boards separately, or link one
  board's fragment directly as another catalog's `subitems_url`).
- `os_list.json` — a combined catalog listing every image built in this
  invocation.

`--publish-base-url` is required whenever `--catalog` is given — gosd
refuses to guess where you're going to host the files, since every entry's
`url` field is `--publish-base-url` joined with the image's filename. If
you omit it, `gosd build` fails immediately, before doing any building, with
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
  filename (this is what `os_list.json`'s `url` fields already point at, so
  as long as you upload the files gosd wrote to the location you passed as
  `--publish-base-url`, this is automatic).
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

Sending this URL to non-technical end users? Send them to
[`docs/flashing.md`](flashing.md) instead of this page — it walks through
the same steps above with screenshots and no jargon, and includes a
copy-paste snippet for your own README.

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
