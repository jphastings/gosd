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

Written next to the built image whenever `--placeholder` was given (the
extension is swapped, the same convention `--catalog`'s `os_list.json`
fragments use):

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

## Client algorithm (typical shape)

A tool consuming this manifest to splice in real configuration should:

1. Obtain the manifest from a source it trusts (its own origin, a
   signed/pinned URL — the manifest itself carries no signature).
2. Render the real configuration for each placeholder, padded to exactly
   that placeholder's `size`; refuse up front if the real content wouldn't
   fit.
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
