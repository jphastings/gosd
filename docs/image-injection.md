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

## Injecting environment variables

Your app's environment variables are the one setting this mechanism can't
reach directly. They come from two places only (see
[how an app receives its environment](runtime.md#app-environment-variables-gosdtoml-env)):
defaults baked into `config.json`, which lives inside the compressed
initramfs and so has no stable byte ranges, and the card's `gosd.toml [env]`
table — and `gosd.toml`, like every other existing boot file, is refused as
a `--placeholder` path. Cloud-init provisioning carries a hostname and WiFi
credentials, never environment variables.

Reserve a placeholder your app reads for itself instead. The boot partition
stays mounted read-only at `/boot` while your app runs (see
[the storage tiers](runtime.md#root-filesystem-ram-wiped-every-reboot)), so
a placeholder is an ordinary file it can open at startup:

```sh
gosd build . --board pi-zero-2w --placeholder app.env=4KiB
```

Fill it in like any other placeholder. Content shorter than the reserved
size is padded with trailing newlines, so pick a format that tolerates
trailing whitespace — JSON does, and escapes any value you can throw at it:

```ts
await withPlaceholders("https://dl.example.com/myapp-pi-zero-2w.img", {
  "app.env": JSON.stringify({ API_URL: "https://api.example.com", API_TOKEN: token }),
});
```

Then read it back before anything consults the environment. Setting the
values with `os.Setenv` keeps every `os.Getenv` call site in your app
unchanged, whether the values were injected, baked with `--env`, or
hand-written into `gosd.toml`:

```go
// loadInjectedEnv applies the settings a provisioning tool spliced into a
// placeholder file on the boot partition. A missing file, or one no tool has
// filled in, leaves the environment untouched.
func loadInjectedEnv(path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if bytes.HasPrefix(raw, []byte("# GOSD-PLACEHOLDER")) {
		return nil
	}

	var injected map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(raw), &injected); err != nil {
		return fmt.Errorf("reading injected settings from %s: %w", path, err)
	}
	for key, value := range injected {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
```

Called as `loadInjectedEnv("/boot/app.env")` at the top of `main`. If a
dependency reads its configuration in a package `init` function, hand it the
values directly rather than relying on `os.Setenv` running first.

Three differences from a real `gosd.toml [env]` value are worth knowing
before choosing this route:

- **Crash-report redaction isn't automatic.** `gosd-init` turns every
  environment value it merges into a redaction rule, so an `[env]` secret
  never reaches a crash report; values your app loads for itself are
  invisible to it. Register the secret ones as soon as you hold them, with
  `fault.RegisterSecretString(os.Getenv("API_TOKEN"), "api-token")` (see
  [what a crash report does with secrets](crash-reports.md#secrets)) — and
  only the secret ones, since a registered value is replaced everywhere it
  appears, including in the technical detail you wanted to read.
- **The reserved names aren't policed.** `gosd build --env` refuses a
  `GOSD_*` key and `gosd-init` ignores one written into `gosd.toml`, but
  nothing stops your own `os.Setenv` from overwriting `GOSD_BOARD` or
  `GOSD_HOSTNAME` with an injected value. Don't.
- **It's plaintext on the boot FAT**, exactly like a `gosd.toml [env]`
  value or a WiFi passphrase: anyone holding the card can read it.

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
