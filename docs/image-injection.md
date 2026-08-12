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
`--config-placeholder` was given (the extension is swapped, the same convention
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
  "config": {
    "path": "gosd.toml",
    "size": 16384,
    "sha256": "<hex, gosd.toml as gosd built it>",
    "ranges": [ { "offset": 17334272, "length": 16384 } ],
    "pristine": "# These are the settings for this device.\n..."
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
- `config` is present only on a build that passed `--config-placeholder`, and
  describes the card's `gosd.toml`: the same fields a placeholder has, plus
  `pristine` — the region's exact text, so a client can edit the config it
  was given instead of reconstructing one. Hashing `pristine` reproduces
  `sha256`, which is how a client proves the text it was handed is the text
  on the card. See "Injecting configuration" below; a client that predates
  the key ignores it and keeps working.

## Client algorithm (typical shape)

A tool consuming this manifest to splice in real configuration should:

1. Obtain the manifest from a source it trusts (its own origin, a
   signed/pinned URL — the manifest itself carries no signature).
2. Render the real content for each placeholder — and for the `config`
   region, if the manifest has one, editing its published `pristine` text or
   replacing it — padded to exactly the `size` declared for it; refuse up
   front if it wouldn't fit.
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

## Injecting configuration: hostname, WiFi, settings, ingress

The card's `gosd.toml` holds everything a device is provisioned with — its
hostname, WiFi credentials, the app's `[env]` settings, and the
[`[ingress.*]`](ingress.md) tunnel it exposes itself through. `gosd build
--config-placeholder` pads that file out to a fixed size and publishes it in
the manifest, so a downloader can rewrite the whole thing:

```sh
gosd build . --board pi-zero-2w \
  --env API_URL=https://api.example.com \
  --config-placeholder
```

The flag takes an optional size (`--config-placeholder 32KiB`); given without
one it reserves 16KiB, which is several times what the file renders to.

What a downloader writes there is an ordinary `gosd.toml`, and that is the
whole point of reserving the file rather than shipping a second one of gosd's
own:

- **No app code.** `gosd-init` reads the file it always reads. Settings reach
  your app's environment, WiFi credentials reach the network bring-up, a
  tunnel token reaches the ingress agent.
- **It survives a reflash.** The
  [provisioning snapshot](runtime.md#the-provisioning-snapshot-surviving-a-reflash)
  treats injected values as the operator's own — because on the card, they
  are — and restores them onto the next image flashed over the top. Ingress
  sections are restored as a whole unit, so a per-device tunnel keeps working
  after an upgrade.
- **Crash reports redact it.** Every `[env]` value gosd-init merges becomes a
  redaction rule.
- **The card still explains itself**, provided you edit the file rather than
  replace it — see below.

### What to write into the region

Exactly `size` bytes of valid TOML, padded to length with newlines. The
manifest publishes the region's **pristine text** alongside its hash, so the
natural move is to edit what you were given: uncomment the
`[ingress.cloudflared]` block and fill in the token, change `hostname`, add a
key under `[env]`. Everything gosd wrote for whoever opens the card — the
plain-language guidance above each setting — then survives into the flashed
image.

Replacing the text wholesale is legal too, and the file is yours if you do:
anything you leave out (including defaults baked with `--env`) is simply
absent, and the card's guidance goes with it. Either way the result must
parse as TOML; a file that doesn't is a file `gosd-init` falls back from,
logging loudly (see [gosd.toml](gosd.toml.md)).

### What happens on the next reflash

Reflashing rewrites the whole boot partition, `gosd.toml` included. On a
`--data-size=expand` image, `gosd-init` keeps a snapshot in `/data` of what
each boot settled on, and the first boot after a reflash decides each setting
on its own:

1. **What the new card says wins** — whether a tool injected it or a person
   typed it, both are the same file. Re-provisioning a device works, and so
   does hand-editing one.
2. **Otherwise the snapshot restores** what this device settled on before,
   provided that value differed from the baked default it was measured
   against — the proof somebody chose it. This is the plain-reflash case: an
   image flashed with no injection at all gets the device's previous
   hostname, WiFi, settings and tunnel back.
3. **Otherwise the newly flashed image's baked defaults apply**, exactly as
   on a first flash.

Recovery of any kind presupposes `/data` survived the reflash — an
`--data-size=expand` image whose on-card ABI (`--boot-size`,
`--data-filesystem`, `--label-prefix`) hasn't changed. No data partition
means no snapshot: injected settings are simply gone, replaced by the new
image's defaults.

### Secrets

Injected credentials — a WiFi passphrase, a tunnel token, an API key — are
redacted from crash reports automatically, but they are **plaintext on the
boot FAT**, and because the snapshot carries them, a copy also lives in
`/data` and is re-rendered into the *next* card's `gosd.toml` on an upgrade.
A reflash is therefore not a way to wipe a device's credentials; only
clearing `/data` is.

### Settings that aren't configuration

An app whose own configuration is a YAML document, a key file, or anything
else that isn't a gosd.toml setting still wants an ordinary `--placeholder`
file, read from `/boot` at startup — the boot partition stays mounted
read-only while the app runs. What that route doesn't get is any of the
properties above: nothing on the device knows the file exists, so it's lost
on reflash unless the app persists it to `/data` itself.

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
