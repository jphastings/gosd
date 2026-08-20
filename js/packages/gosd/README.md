# gosd

Download a [GoSD](https://github.com/jphastings/gosd)-built SD-card image, verify it end to
end, and splice per-deployment configuration into its reserved placeholder files as the bytes
stream past — one call, no FAT32 code, no buffering the whole image unless you have to.

This is the JavaScript/TypeScript half of GoSD's [image-injection
contract](https://github.com/jphastings/gosd/blob/main/docs/image-injection.md):
`gosd build --placeholder <path>=<size>` reserves fixed-size files on the image's boot
partition and writes a `<image>.inject.json` manifest recording their exact byte ranges. This
package fetches that manifest, fetches the image, verifies both against it, and rewrites the
declared ranges in place while the image is still streaming to disk.

```sh
npm install @jphastings/gosd
```

## Quickstart

```ts
import { withPlaceholders } from "@jphastings/gosd/downloads";

// Call this directly from a click handler, not after an earlier `await` —
// see "Save tiers" below for why.
async function onDownloadClick() {
  const result = await withPlaceholders("https://dl.example.com/app-rock-4se.img", {
    "backupist.yaml": renderConfigYaml(userInput),
    "network-config": renderNetworkConfig(wifi),
  });
  console.log(`saved via ${result.savedVia}, sha256 ${result.sha256}`);
}
```

`files` is a map from placeholder path (as declared by `--placeholder` at build time) to its
replacement content — a `string` (UTF-8 encoded) or a `Uint8Array`. You don't have to fill
every placeholder the image has; the rest are left exactly as `gosd build` rendered them
(read as absent, per the contract), but their bytes are still hash-verified. Content shorter
than its placeholder is padded to the exact reserved size with trailing newlines (`0x0A`) —
harmless to the text formats placeholders carry — and content that doesn't fit fails before
anything downloads.

## Device settings (`options.config`)

Every gosd-built image carries a `config/` directory on its boot partition: one setting per
file, which the person holding the card can edit in any text editor. `options.config` fills
those same files in as the image downloads, keyed by their path within that directory:

```ts
await withPlaceholders(
  imageURL,
  {},
  {
    config: {
      "wifi/ssid": ssid,
      "wifi/passphrase": passphrase,
      "env/API_TOKEN": token,
    },
  },
);
```

What lands there is exactly what someone would have typed into that file by hand, so the
device treats it as its own setting — and, because the setting lives on the card rather than
inside the app, it survives a later reflash. Which settings an image has is up to the app it
was built from; `manifest.config` lists them, each with the value it currently ships with
(`""` means unset) and the size it reserves.

A value is padded to that reservation with trailing newlines, exactly as gosd pads the
pristine file. The reservation is fixed at build time and can never grow afterwards, so
anything that must accept a long value (an API token, a tunnel token) ships with the room
already held open.

Refused before anything downloads: an image with no settings at all, a path this image
doesn't have (the error lists the ones it does), a value longer than its reservation, a
non-string value, and an `env/` name that is unusable or in the `GOSD_*` namespace gosd
reserves for itself (the device would ignore it).

`files` and `config` compose freely — an image can carry both, and every region is
hash-verified whether or not you fill it in.

## Threat model

The download host is **untrusted**. Only the manifest is trusted (fetched same-origin, or
pinned via `options.manifestSha256`). Concretely:

- The whole image is hashed with a real streaming SHA-256 as it downloads and compared
  against `manifest.image.sha256` — **always**, even if the server's `ETag` already looks
  like a match. A matching `ETag` only lets a corrupt response fail _fast_ (before writing
  anything); it never lets a corrupt response fail _silently_.
- Every placeholder's current bytes are hashed and compared against its manifest entry the
  instant they're fully read, whether or not you're patching that placeholder — proving the
  image wasn't tampered with (or already patched by something else) before a single
  replacement byte is written.
- A save fails closed: the fs-access and memory tiers never leave a file behind on failure
  (see "Save tiers"); a bit flipped anywhere in the image aborts the whole download.

- The manifest is read with a hard size cap rather than buffered whole: a pin only makes a
  bad manifest _detectable_, and detecting it is no use if the tab has already run out of
  memory taking delivery of it. Fetching an unpinned manifest over plain `http://` (loopback
  aside) logs a `console.warn`, because that is the one fetch here whose integrity nothing
  downstream can re-derive.

No secret data is ever hashed here — image bytes and the manifest are both meant to be
public — so this package doesn't attempt to be constant-time; that would be solving a
problem that doesn't exist here at the cost of real streaming performance.

### Escaping is yours, not ours

Every value you pass in `files` or `config` is written to the card **verbatim** — a
placeholder is a whole pre-rendered file, and a setting is exactly what someone would have
typed into it — so this package has no template to inject into and nothing to escape on your
behalf. That also means the quickstart's `renderConfigYaml(userInput)` is where the
responsibility lives: if you interpolate user-supplied text into YAML, JSON, or an `env/`
value, escape it there with a real serializer for that format. Text the device then parses as
structure it wasn't meant to be is a bug in the rendering, and one this library cannot see.

## Save tiers

`withPlaceholders` auto-selects the best mechanism this browser supports, in order, and
demotes to the next one (with a `console.warn`) if the preferred one fails for a reason other
than the user cancelling:

| Tier                         | Mechanism                                                                                                   | Peak memory             | Real progress UI                 | Support                                                                                                                                                                        |
| ---------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1. `fs-access`               | [File System Access API](https://developer.mozilla.org/en-US/docs/Web/API/File_System_API) streaming writes | ~one chunk              | Yes (native save dialog)         | Chromium (Chrome, Edge, Opera, Arc). Not Firefox, not Safari.                                                                                                                  |
| 2. `service-worker` (opt-in) | A same-origin service worker synthesizes a real `Response` the browser downloads                            | ~one chunk              | Yes (browser's download manager) | Chrome, Firefox. Safari: falls back to a slow chunk-by-chunk pump, and its download manager doesn't show progress reliably — steer Safari users to tier 3 instead (see below). |
| 3. `memory`                  | Buffer everything, then a `Blob` + anchor-click download                                                    | ~2× image size, briefly | No                               | Every evergreen browser. Always available; it's the guaranteed fallback.                                                                                                       |

Force a specific tier with `options.saveVia` — a forced tier's failure **throws** instead of
falling back, since you asked for it explicitly.

### Tier 1: File System Access (`fs-access`)

Auto-selected whenever `window.showSaveFilePicker` exists. **Call `withPlaceholders` directly
from your click handler**, with no `await` before it: `showSaveFilePicker` needs to run
inside the same synchronous call chain as the user gesture that triggered it (a JS `async`
function runs synchronously up to its first `await`, and this package calls the picker as
that first thing). If you need to do async setup first — e.g. render the config from a form —
finish that, then call `withPlaceholders` as a direct, un-awaited-before reaction to the
click.

Chromium stages writes to a swap file that's only published to the real path when the stream
closes, so a failure partway through never leaves a truncated file at the user-visible path.
If the user dismisses the picker, `withPlaceholders` rejects with `GosdCancelledError` — at
that point nothing has been fetched yet. This is the only tier that can resume an interrupted
download later — see "Resuming" below.

### Tier 2: Service worker (`service-worker`, opt-in)

Not auto-selected unless you pass `options.serviceWorker = { url, scope }`. This package
ships an import-free worker script at the `@jphastings/gosd/downloads/service-worker.js` export subpath
— **you host it yourself**, same-origin:

```ts
// e.g. copy dist/sw/gosd-download-sw.js from node_modules/@jphastings/gosd to your
// public/ directory as part of your build, or serve it directly if your
// bundler supports `?url` imports of package subpaths.
await navigator.serviceWorker.register("/gosd-download-sw.js", { scope: "/" });

await withPlaceholders(imageURL, files, {
  serviceWorker: { url: "/gosd-download-sw.js", scope: "/" },
});
```

The worker synthesizes a `Response` with `Content-Length` (from the manifest, so the browser
shows real progress) and `Content-Disposition: attachment`, served from a same-origin URL
under its scope (`<scope>gosd/<id>/<filename>`) that `withPlaceholders` triggers via a hidden
iframe. Aborting errors that stream, which makes the browser mark the download failed and
clean up its own partial file.

**Honest browser support:** Chrome and Firefox transfer the patched bytes to the worker as a
streamed `ReadableStream` with no extra copying. Safari doesn't support transferable streams,
so there this tier falls back to pumping chunks over `postMessage` one at a time — it works,
but it's slower and Safari's download UI doesn't reliably reflect progress. If your users are
predominantly on Safari, prefer the memory tier (`saveVia: "memory"`) instead of forcing this
one. Firefox also aggressively idle-kills service workers (~30s); this package sends a
keepalive ping every 20s while a download is in flight to prevent that.

### Tier 3: Memory (`memory`)

The universal fallback: every chunk is buffered, and **only once the entire image has been
fetched, verified, and patched** does it become a `Blob` and trigger a normal browser
download via a temporary anchor click. Peak memory is roughly twice the image size for a
short window (the buffered chunks, then the `Blob`'s own copy) — fine for typical GoSD images
(tens to low hundreds of MiB), but don't reach for this tier for anything you'd hesitate to
hold in memory twice.

## Errors

Every failure is a `GosdError` subclass with a stable, machine-readable `code`:

| Class                             | `code`                     | When                                                                                                                                  |
| --------------------------------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `GosdManifestFetchError`          | `manifest-fetch`           | The `.inject.json` request failed or returned a non-2xx status.                                                                       |
| `GosdManifestInvalidError`        | `manifest-invalid`         | The manifest doesn't parse as JSON, or fails structural validation (a bad field is named by its JSON path).                           |
| `GosdManifestHashMismatchError`   | `manifest-hash-mismatch`   | The manifest's bytes don't match a caller-pinned `options.manifestSha256`.                                                            |
| `GosdUnknownPlaceholderError`     | `unknown-placeholder`      | A key in `files` doesn't name a placeholder in the manifest.                                                                          |
| `GosdUnknownConfigError`          | `unknown-config`           | A key in `config` doesn't name a setting in this image, or the image has no settings at all.                                          |
| `GosdInvalidEnvError`             | `invalid-env`              | An `env/` setting's name isn't a valid environment variable name, is in the reserved `GOSD_*` namespace, or its value isn't a string. |
| `GosdContentTooLargeError`        | `content-too-large`        | A value in `files` or `config` doesn't fit the size its region reserves.                                                              |
| `GosdImageFetchError`             | `image-fetch`              | The image request failed, returned a non-2xx status, or had no body.                                                                  |
| `GosdImagePreconditionError`      | `image-precondition`       | The response's `ETag` or `Content-Length` disagrees with the manifest, before any body byte was read.                                 |
| `GosdPlaceholderNotPristineError` | `placeholder-not-pristine` | A placeholder's — or a setting's — current bytes don't hash to the manifest's recorded value: tampered, or already patched.           |
| `GosdImageHashMismatchError`      | `image-hash-mismatch`      | The whole streamed image's SHA-256 doesn't match `manifest.image.sha256`.                                                             |
| `GosdImageSizeError`              | `image-size`               | The stream ended short of, or ran past, `manifest.image.size`.                                                                        |
| `GosdSaveFailedError`             | `save-failed`              | The chosen save tier itself failed (e.g. no active service worker, no DOM for the memory tier).                                       |
| `GosdCancelledError`              | `cancelled`                | The user dismissed the fs-access save picker. Nothing had been fetched yet.                                                           |

```ts
import { GosdError } from "@jphastings/gosd/downloads";

try {
  await withPlaceholders(imageURL, files);
} catch (err) {
  if (err instanceof GosdError) {
    console.error(`download failed (${err.code}): ${err.message}`);
  }
  throw err;
}
```

## Node / core API

Everything except the three save tiers and `withPlaceholders` itself runs in plain Node
(22+) — no DOM, no `fetch` polyfill needed beyond what Node already provides globally. This
is what the package's own tests build on, and it's there for anyone assembling their own
pipeline (a CLI provisioning tool, a CI job that pre-patches images, etc.):

```ts
import {
  deriveManifestURL, // image URL -> manifest URL, mirroring gosd's own convention
  fetchManifest, // fetch + parse + optional hash-pin, in one call
  parseManifest, // structural validation of an already-fetched manifest
  padContents, // your { path: content } -> exact-size padded Uint8Arrays
  createSubstitutionTransform, // the core TransformStream<Uint8Array, Uint8Array>
  patchStream, // sugar: source.pipeThrough(createSubstitutionTransform(...))
  checkImageResponse, // the ETag/Content-Length precondition checks
  runDownload, // fetch -> check -> patch -> pipe into any SaveSink -> commit
  Sha256, // the vendored streaming hasher itself
} from "@jphastings/gosd/downloads";
```

A `SaveSink` is just `{ kind, writable, commit(), abort(reason) }` — write your own to drive
`runDownload` against a Node `fs` file handle, an S3 upload, or anything else that can accept
a `WritableStream<Uint8Array>`. A sink can additionally implement `SeekableSaveSink` (checked
with `isSeekable`) to support resuming — see "Resuming" below.

## Resuming

The fs-access tier can resume an interrupted download across a page reload — the other two
tiers can't (a service worker's in-flight state and a buffered `Blob` don't survive one, so
resuming isn't offered for them). Opt in with `options.resumable`:

```ts
async function onDownloadClick() {
  await withPlaceholders(imageURL, files, { resumable: true });
}
```

This checkpoints progress to IndexedDB as the download streams: the `FileSystemFileHandle` the
user picked, the image's identity (`manifest.image.sha256`) and size, the response's
`ETag`/`Last-Modified`, and — as each patched placeholder finishes verifying — its pristine
(pre-substitution) bytes, which are small (placeholders are KiB-scale) and are what a later
resume needs to reconstruct the equivalent original bytes for re-verification, since only
placeholder ranges are ever rewritten on disk. On a full success the checkpoint is deleted; on
a failure that isn't corruption (a network drop, a cancelled request), whatever was durably
streamed is preserved instead of thrown away.

On your next page load, offer to continue:

```ts
import {
  listResumableDownloads,
  resumeDownload,
  discardResumableDownload,
} from "@jphastings/gosd/downloads";

const pending = await listResumableDownloads();
// pending: [{ key, imageURL, filename, imageSize, bytesWritten }, ...]

async function onResumeClick(key: string) {
  try {
    const result = await resumeDownload({ key, files });
    console.log(`resumed and saved via ${result.savedVia}, sha256 ${result.sha256}`);
  } catch (err) {
    // The partial file failed re-verification, or permission to keep
    // writing to it was refused — start over instead.
    await discardResumableDownload(key);
    await withPlaceholders(pending.find((p) => p.key === key)!.imageURL, files);
  }
}
```

`resumeDownload` re-verifies whatever's already on disk (re-hashing a reconstruction of its
pristine bytes rather than serializing the vendored `Sha256`'s internal state — there's no
resumable-hash format to keep in sync across versions this way) before issuing an HTTP `Range`
request with `If-Range` pinned to the stored `ETag`/`Last-Modified`, and continues the same
substitution/verification pass a fresh download would have run. If the server ignores the
`Range` (a plain `200` — no support, or the resource changed), it restarts from scratch, reusing
the same already-picked file so the save picker never reappears.

Resuming needs an explicit `key` — from `listResumableDownloads`, or `manifest.image.sha256` if
you already have the manifest — because the whole point is to act _before_ re-running
`withPlaceholders` (which would show the save picker again). `withPlaceholders` itself never
auto-resumes.

## Zero runtime dependencies

This package has none. The one piece the browser platform doesn't provide — incremental
SHA-256 hashing over a stream (`crypto.subtle.digest` only hashes a value already fully in
memory) — is a small vendored implementation
([`src/downloads/sha256.ts`](src/downloads/sha256.ts)), pinned by NIST CAVP test vectors and
cross-checked against `crypto.subtle.digest` on random inputs.
