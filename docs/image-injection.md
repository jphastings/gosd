# Image injection: filling in configuration after the image is built

`gosd build --placeholder <path>=<size>` (repeatable) reserves fixed-size,
comment-padded placeholder files on the boot partition and writes a
`<image>.inject.json` manifest next to each built image recording the
absolute byte ranges each placeholder's content occupies in the `.img`. A
downstream tool — typically a browser splicing configuration into an image
between the CDN and the user's disk — can then overwrite those ranges with
same-length bytes via a plain positional write, with no FAT32 code at all,
and the file reads back patched at the filesystem level.

This is for developers distributing a per-user or per-deployment config
(API keys, WiFi credentials, a device identity) without building a
different image for every recipient. If your app's configuration is the
same for everyone, you don't need this — bake it with `--env` or a
hand-edited `gosd.toml` instead (see `docs/publishing.md`).

## Why this works

A FAT file's *content* lives only in the data region of the partition; its
size and location are recorded separately, in a directory entry and the FAT
tables. Overwriting a placeholder's content ranges with **exactly the same
number of bytes** changes the file's content without touching any
filesystem structure. Nothing at boot checksums the boot partition's contents
(gosd-init only requires that `gosd.toml` exists and parses), so the patch
is invisible to everything except whatever reads that specific file — which
is the point.

## Placeholder file contract

- Rendered deterministically at build time — the image identity stays
  reproducible (`gosd build`'s content-derived identity, see
  `docs/runtime.md`) — exactly `size` bytes, final byte `\n`.
- First line: `# GOSD-PLACEHOLDER v1 path=<path>` — e.g.
  `# GOSD-PLACEHOLDER v1 path=backupist.yaml`.
- Followed by a short human-readable explanation, then `#`-comment padding
  to the exact requested size. The whole file is valid YAML (comments and
  blank lines only), so it's legible to anyone who mounts the card and
  looks.
- **Consumers should treat a file whose content still starts with
  `# GOSD-PLACEHOLDER` as absent.** A pristine (unpatched) placeholder then
  behaves exactly like a missing file — e.g. a comment-only `network-config`
  cloud-init file unmarshals to an empty document, which gosd-init already
  treats as "no WiFi seed provided" (see `docs/provisioning-formats.md`).

Placeholders join the FAT root's other files (`gosd.toml` included) before
`gosd build` computes the image's content-derived identity, so they're
covered by it exactly the same way — an identical `--placeholder` flag set
across rebuilds keeps the identity reproducible. A `--placeholder` path
that collides with an existing boot file, `gosd.toml`, or another
placeholder is refused case-insensitively (FAT paths are case-insensitive).

## The manifest: `<image basename>.inject.json`

Written next to the built image whenever `--placeholder` or
`--env-placeholder` was given (the extension is swapped, the same convention
`--catalog`'s `os_list.json` fragments use):

```json
{
  "gosd_inject": 1,
  "board": "pi-zero-2w",
  "image": {
    "filename": "myapp-pi-zero-2w.img",
    "size": 285212672,
    "sha256": "<hex, the whole pristine image>"
  },
  "placeholders": [
    {
      "path": "backupist.yaml",
      "size": 32768,
      "sha256": "<hex, the placeholder's rendered content>",
      "ranges": [ { "offset": 17301504, "length": 32768 } ]
    }
  ],
  "env": {
    "size": 8192,
    "sha256": "<hex, the reserved region as gosd built it>",
    "ranges": [ { "offset": 17334272, "length": 8192 } ]
  }
}
```

- `offset`/`length` are bytes, absolute within the image file.
- `ranges` is ordered; a placeholder's content is the concatenation of its
  ranges in order. A freshly formatted boot partition allocates contiguously,
  so one range per placeholder is the norm, but a consumer must handle
  several — fragmentation is legal FAT32, even if unlikely here.
- The sum of a placeholder's `ranges[].length` always equals its `size`;
  every range lies inside the boot partition. `gosd build` guarantees both;
  a careful consumer verifies them too.
- `placeholders[].sha256` lets a consumer prove a placeholder's ranges are
  still pristine (unpatched) before writing to them, independent of the
  whole-image hash.
- `env` is present only on a build that passed `--env-placeholder`, and
  describes a region *inside* `gosd.toml` rather than a file of its own —
  same three fields, same guarantees, and the same
  `# GOSD-PLACEHOLDER`-style pristine check via its `sha256`. It's a
  separate key precisely because it isn't a whole file: writing its `size`
  bytes anywhere but its `ranges` would corrupt the rest of gosd.toml. See
  "Injecting environment variables" below. A client that predates the key
  ignores it and keeps working.

## Client algorithm (typical shape)

A tool consuming this manifest to splice in real configuration should:

1. Obtain the manifest from a source it trusts (its own origin, a
   signed/pinned URL — the manifest itself carries no signature).
2. Render the real configuration for each placeholder — and for the `env`
   region, if the manifest has one — padded to exactly the `size` declared
   for it; refuse up front if the real content wouldn't fit.
3. Fetch the image; verify its SHA-256 against `image.sha256`.
4. Verify each placeholder's currently-pristine ranges hash to
   `placeholders[].sha256`, proving nothing unexpected has changed there.
5. Splice the padded content into the declared ranges with a plain
   positional write (`WriteAt`/`pwrite`/a Blob slice — whatever the
   platform's equivalent is); save.

No step requires understanding FAT32, MBR partitioning, or any other part
of the image format — every byte range needed is already in the manifest.
For browser/Node JavaScript, the official `@jphastings/gosd` npm package's
`@jphastings/gosd/downloads` subpath (`js/packages/gosd` in this repo) implements this
whole algorithm — fetch, verify, splice, save — behind one call,
`withPlaceholders(imageURL, files)`; see its README for the quickstart,
threat model, and save-tier details. A worked implementation (browser-side,
spliced entirely client-side between the CDN and the user's disk) also
lives in the Backup.ist project's `docs/IMAGE-INJECTION.md`.

## Injecting environment variables

Most per-device configuration is a handful of settings the app reads from its
environment, so those have a mechanism of their own.
`gosd build --env-placeholder <size>` reserves that many bytes for the body of
gosd.toml's `[env]` table and publishes that region's byte ranges in the
manifest, under a top-level `env` key:

```sh
gosd build . --board pi-zero-2w \
  --env API_URL=https://api.example.com \
  --env-placeholder 8KiB
```

A downloader overwrites those ranges the same way it overwrites a
placeholder's, and what it writes becomes an ordinary `gosd.toml` `[env]`
value. That is the whole point of reserving space *inside* gosd.toml rather
than shipping a file of gosd's own:

- **No app code.** `gosd-init` merges the region into your app's environment
  along with everything else, so `os.Getenv` finds it (see
  [how an app receives its environment](runtime.md#app-environment-variables-gosdtoml-env)).
- **It survives a reflash**, through the same provisioning snapshot that
  carries hand-edited settings across an upgrade — see below.
- **Crash reports redact it.** Every value `gosd-init` merges becomes a
  redaction rule, so an injected API token can't reach
  `LAST_FATAL_ERROR.md`.
- **The card still explains itself.** Someone who opens gosd.toml sees the
  injected settings where every other setting lives, and can edit them.

### What to write into the region

Exactly `size` bytes of TOML `[env]` **body** — `KEY = "value"` lines and
comments — padded to length with newlines. Two rules:

- **No section headers.** The region sits inside the `[env]` table, so a
  `[wifi]` or `[anything]` line would capture every setting gosd wrote after
  it.
- **Restate every key you want.** The region is the whole body, not an
  addition to it, and a key given twice in one TOML table is a parse error
  rather than a last-one-wins override. Whatever you leave out falls back to
  the image's baked default.

A pristine region isn't "absent" the way an untouched placeholder file is: it
holds the `--env`/`--env-file` defaults this image was built with, rendered
exactly as they would be without `--env-placeholder`, plus comment padding.
An un-injected card therefore behaves as if the flag had never been passed.

### What happens on the next reflash

Reflashing rewrites the whole boot partition, gosd.toml included. On a
`--data-size=expand` image, `gosd-init` keeps a
[provisioning snapshot](runtime.md#the-provisioning-snapshot-surviving-a-reflash)
in `/data` of what each boot settled on, and the first boot after a reflash
decides each `[env]` key on its own:

1. **What the new card says wins** — whether a tool injected it or a person
   typed it, both are the same file. So re-injecting a device with new
   settings works, and so does hand-editing one.
2. **Otherwise the snapshot restores** what this device settled on before,
   provided that value differed from the baked default it was measured
   against — the proof somebody chose it. This is the plain-reflash case: an
   image flashed with no injection at all gets the device's previous settings
   back.
3. **Otherwise the newly flashed image's baked default applies**, exactly as
   on a first flash.

Consequence worth knowing: re-injecting overrides a hand-edit made on the
previous card, because a freshly injected card is the newer statement of
intent. And recovery of any kind presupposes `/data` survived the reflash —
an `--data-size=expand` image whose on-card ABI (`--boot-size`,
`--data-filesystem`, `--label-prefix`) hasn't changed. No data partition
means no snapshot: the injected values are simply gone, replaced by the new
image's defaults.

### Secrets

An injected secret is redacted from crash reports automatically, but it is
still **plaintext on the boot FAT**, exactly like a WiFi passphrase — and
because the snapshot carries it, a copy also lives in `/data` and is
re-rendered into the *next* card's gosd.toml on an upgrade. A reflash is
therefore not a way to wipe a device's injected credentials; only clearing
`/data` is.

### Settings that aren't environment variables

An app whose configuration is a YAML document, a key file, or anything else
that doesn't reduce to `KEY=value` still wants an ordinary `--placeholder`
file, read from `/boot` at startup — the boot partition stays mounted
read-only while the app runs. That was the only way to inject anything
app-facing before `--env-placeholder` existed, and it still works; what it
doesn't get is any of the four properties above, so a file read this way is
lost on reflash unless the app persists it to `/data` itself.

## Imager compatibility

An image built with `--placeholder` stays fully compatible with Raspberry
Pi Imager's custom-repository flow (`docs/publishing.md`): Imager's own
customization wizard writes real files (`gosd.toml`, `user-data`, etc.)
over the placeholders using the OS's normal FAT driver, exactly as it would
on an image with no placeholders at all. The two mechanisms don't
interact — a placeholder is just an ordinary file until something,
Imager included, decides to overwrite it.

## Example

```sh
gosd build . --board pi-zero-2w \
  --placeholder backupist.yaml=32KiB \
  --placeholder network-config=4KiB \
  -o myapp.img
```

writes `myapp.img` and `myapp.inject.json` side by side. `32KiB` and `4KiB`
are just examples — size each placeholder for the largest config you'll
ever need to inject; the file can't grow at patch time, only be
overwritten in place.
