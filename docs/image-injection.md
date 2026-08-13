# Image injection: filling in configuration after the image is built

`gosd build --placeholder <path>=<size>` (repeatable) reserves fixed-size,
comment-padded placeholder files on the boot partition and writes a
`<image>.inject.json` manifest next to each built image recording the
absolute byte ranges each placeholder's content occupies in the `.img`. A
downstream tool — typically a browser splicing configuration into an image
between the CDN and the user's disk — can then overwrite those ranges with
same-length bytes via a plain positional write, with no FAT32 code at all,
and the file reads back patched at the filesystem level.

Every image also carries a `config/` directory on its boot partition — one
setting per file (`wifi/ssid`, `env/API_TOKEN`, ...), each padded to a
reservation of its own — and the same manifest publishes those files' byte
ranges too, so a downloader fills settings in exactly the way it fills in a
placeholder.

This is for developers distributing a per-user or per-deployment config
(API keys, WiFi credentials, a device identity) without building a
different image for every recipient. If your app's configuration is the
same for everyone, you don't need this — ship the value in your app's own
`config/` directory (`gosd build --config-dir`) instead.

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

Written next to every built image (the extension is swapped, the same
convention `--catalog`'s `os_list.json` fragments use):

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
  "config": [
    {
      "path": "env/API_TOKEN",
      "size": 256,
      "sha256": "<hex, the value file as gosd wrote it>",
      "ranges": [ { "offset": 17334272, "length": 256 } ],
      "value": ""
    }
  ]
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
- `config[]` lists every value file in the image's `config/` tree, keyed by
  its path within that directory (no leading `config/`). It carries a
  placeholder's four fields plus `value`, the setting's current contents
  newline-trimmed — `""` for an unset one — so a tool can show what the
  image ships with, and tell a set default from an unset one, without
  parsing a FAT filesystem for bytes that are already public in the `.img`
  it is about to download. `size` is the file's reservation, not the length
  of `value`. See "Injecting settings" below.

## Client algorithm (typical shape)

A tool consuming this manifest to splice in real configuration should:

1. Obtain the manifest from a source it trusts (its own origin, a
   signed/pinned URL — the manifest itself carries no signature).
2. Render the real configuration for each placeholder and setting, padded to
   exactly the `size` declared for it; refuse up front if the real content
   wouldn't fit.
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

## Injecting settings

Most per-device configuration is a handful of settings — a WiFi network, an
API token — and those live in the image's `config/` tree, one value per
file, rather than in a placeholder of a developer's own. The manifest's
`config` array publishes each of those files exactly as it publishes a
placeholder:

```sh
gosd build . --board pi-zero-2w --config-dir ./config
```

A downloader overwrites a setting's ranges the same way it overwrites a
placeholder's, and the card then reads exactly as if someone had typed the
value into that file by hand:

- **No app code.** A setting under `env/` reaches the app's environment; the
  rest configure the device itself.
- **The card still explains itself.** Every setting ships beside a
  `<name>.explain.md` describing, in plain language, what it does — so
  whoever holds the card can see an injected value where every other
  setting lives, and change it.

### What to write into a setting

Exactly `size` bytes: the setting's text padded to length with trailing
newlines, the same padding gosd writes. Values are read newline-trimmed, so
the padding is invisible, and a file of nothing but newlines reads as unset.

The reservation is fixed when the image is built and can never grow
afterwards, so a setting that has to accept a long value ships as a longer
file: gosd's own `ingress/cloudflared/token` reserves a kilobyte, and an
app's `--config-dir` can ship any value file at whatever size it needs (a
file of 4096 newlines reserves 4KiB and still reads as unset).

### Secrets

An injected secret is **plaintext on the boot FAT**, exactly like a WiFi
passphrase: anyone who can put the card in a computer can read it. Inject
one only where that is an acceptable trade, and prefer a credential you can
revoke.

### Settings that aren't settings

An app whose configuration is a whole YAML document or a key file — anything
that isn't one value a person could sensibly type into a file — still wants
an ordinary `--placeholder`, read from `/boot` at startup (the boot
partition stays mounted read-only while the app runs). What a placeholder
doesn't get is any of the properties above: it is a file gosd knows nothing
about, so it is lost on reflash unless the app persists it to `/data`
itself.

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
