// The client side of gosd's image-injection contract: fetching and
// structurally validating `<image>.inject.json`, and deriving its URL from
// an image URL. See docs/image-injection.md (in the gosd repo) for the
// contract this mirrors — internal/inject.Manifest is the Go source of
// truth for the JSON shape, and internal/inject.ManifestPath for the URL
// derivation. Parsing is structural validation only: every field is
// checked and a bad one throws a descriptive GosdManifestInvalidError
// naming its JSON path, since a malformed manifest means a broken build,
// not something to silently paper over.

import {
  GosdManifestFetchError,
  GosdManifestHashMismatchError,
  GosdManifestInvalidError,
} from "./errors.js";
import { Sha256 } from "./sha256.js";

export interface ByteRange {
  offset: number;
  length: number;
}

export interface PlaceholderInfo {
  path: string;
  size: number;
  sha256: string;
  ranges: ByteRange[];
}

export interface ImageInfo {
  filename: string;
  size: number;
  sha256: string;
}

/** One value file in the image's `config/` tree: a setting a card's owner
 * can edit by hand, and a provisioning tool can fill in as the image
 * downloads. Same shape as a placeholder, plus the value the file currently
 * reads as. */
export interface ConfigInfo {
  /** The setting's path within the tree, with no leading `config/`:
   * `"wifi/ssid"`, `"env/API_TOKEN"`. */
  path: string;
  /** The file's reservation in bytes — what a replacement is padded to. */
  size: number;
  sha256: string;
  ranges: ByteRange[];
  /** What the file reads as today, newline-trimmed; `""` means unset. */
  value: string;
}

export interface Manifest {
  gosd_inject: 1;
  board: string;
  image: ImageInfo;
  placeholders: PlaceholderInfo[];
  /** Every setting in the image's config tree; empty for an image built
   * without one. */
  config: ConfigInfo[];
}

/** One patchable, hash-verified span of the image: a placeholder file, or a
 * config tree value file. Everything downstream of the manifest — padding,
 * substitution, resume — works in these terms, so a setting gets the same
 * verification and resume behaviour as a placeholder without a second code
 * path. */
export interface RegionInfo {
  /** How this region is keyed in the padded-content and captured-pristine
   * maps: a placeholder's FAT path, or a setting's path on the card
   * (`configRegionKey`). */
  key: string;
  /** How the region is named in an error message. */
  label: string;
  size: number;
  sha256: string;
  ranges: ByteRange[];
}

/** The directory the config tree occupies at the root of the card's boot
 * partition — mirrors gosd's own `configtree.Dir`. */
export const CONFIG_DIR = "config";

/** How a setting is keyed in the padded-content and captured-pristine maps:
 * its real path on the card, which no placeholder can also claim (gosd
 * refuses a `--placeholder` colliding with a file the image already has). */
export function configRegionKey(path: string): string {
  return `${CONFIG_DIR}/${path}`;
}

/** Every injectable region of `manifest`, placeholders first. */
export function injectableRegions(manifest: Manifest): RegionInfo[] {
  const regions: RegionInfo[] = manifest.placeholders.map((p) => ({
    key: p.path,
    label: `placeholder "${p.path}"`,
    size: p.size,
    sha256: p.sha256,
    ranges: p.ranges,
  }));
  for (const c of manifest.config) {
    regions.push({
      key: configRegionKey(c.path),
      label: `setting "${c.path}"`,
      size: c.size,
      sha256: c.sha256,
      ranges: c.ranges,
    });
  }
  return regions;
}

/** Derives the injection manifest URL for an image URL: the last path
 * segment's extension (Go `filepath.Ext` semantics — the substring from its
 * final `.` onward, or "" if it has none) is swapped for `.inject.json`.
 * `archive.img.gz` → `archive.img.inject.json`; a segment with no dot gets
 * `.inject.json` appended; `.img` (a dotfile, no other dot) → the whole
 * segment is the "extension", so it becomes just `.inject.json`. The query
 * string is preserved; the fragment is dropped (it's meaningless for a
 * fetch). */
export function deriveManifestURL(imageURL: string | URL): URL {
  const url = new URL(imageURL);
  const segments = url.pathname.split("/");
  const basename = segments[segments.length - 1] ?? "";
  const dotIndex = basename.lastIndexOf(".");
  const trimmed = dotIndex === -1 ? basename : basename.slice(0, dotIndex);
  segments[segments.length - 1] = trimmed + ".inject.json";
  url.pathname = segments.join("/");
  url.hash = "";
  return url;
}

/** The most manifest bytes {@link fetchManifest} will read before giving up.
 *
 * `manifestSha256` makes a tampered manifest *detectable*, but only once its
 * bytes are already in memory — so on its own it is no defence against the
 * same untrusted host answering with an endless body, which would exhaust
 * the tab long before there was anything to hash. A real manifest is a few
 * KiB of JSON; even an image with hundreds of settings stays far inside
 * this, so a response past it is a wrong URL or a hostile one either way. */
export const MAX_MANIFEST_BYTES: number = 1024 * 1024;

export interface FetchManifestOptions {
  /** Defaults to the global `fetch`. Override in tests, or to route through
   * a proxy/cache. */
  fetch?: typeof fetch;
  /** Pins the manifest's own bytes to a known hash (e.g. an index's
   * `inject_sha256`) — checked before parsing. Without it the manifest is
   * trusted the same way any same-origin fetch is. */
  manifestSha256?: string;
  signal?: AbortSignal;
}

/** Fetches and parses an injection manifest. If `manifestSha256` is given,
 * the raw response bytes are hashed and compared before parsing — a
 * mismatch aborts before any JSON is even looked at. */
export async function fetchManifest(
  url: string | URL,
  options: FetchManifestOptions = {},
): Promise<Manifest> {
  const doFetch = options.fetch ?? fetch;
  let response: Response;
  try {
    response = await doFetch(url, { signal: options.signal });
  } catch (cause) {
    throw new GosdManifestFetchError(
      `fetching the injection manifest failed because the request to ${url} could not be sent; check network connectivity and that the manifest URL is correct`,
      { cause },
    );
  }
  if (!response.ok) {
    throw new GosdManifestFetchError(
      `fetching the injection manifest failed because ${url} responded with HTTP ${response.status}; confirm the image and its .inject.json sidecar were both published`,
    );
  }

  const bytes = await readCappedBody(response, url);

  if (options.manifestSha256 === undefined && isOverPlainHTTP(url)) {
    console.warn(
      `gosd: the injection manifest at ${url} was fetched over plain HTTP with no manifestSha256 pin. The image itself is safe over http — every byte is hash-verified against the manifest — but the manifest is what those hashes come from, so anyone on the path can substitute both at once. Serve it over https, or pass manifestSha256 (e.g. your index's inject_sha256).`,
    );
  }

  if (options.manifestSha256 !== undefined) {
    const pin = options.manifestSha256.trim().toLowerCase();
    if (!/^[0-9a-f]{64}$/.test(pin)) {
      throw new GosdManifestHashMismatchError(
        `the manifestSha256 option (${JSON.stringify(options.manifestSha256)}) is not a 64-character hex sha256; pass the manifest file's sha256, e.g. from your index's inject_sha256 field`,
      );
    }
    const digest = new Sha256().update(bytes).digestHex();
    if (digest !== pin) {
      throw new GosdManifestHashMismatchError(
        `the injection manifest at ${url} failed integrity verification: expected sha256 ${options.manifestSha256}, got ${digest}; try re-fetching, or double check the pinned hash you passed`,
      );
    }
  }

  let data: unknown;
  try {
    data = JSON.parse(new TextDecoder().decode(bytes));
  } catch (cause) {
    throw new GosdManifestInvalidError(`the injection manifest at ${url} is not valid JSON`, {
      cause,
    });
  }

  return parseManifest(data);
}

/** Reads the response body, refusing anything past {@link MAX_MANIFEST_BYTES}
 * — by its declared `Content-Length` where there is one, and by counting
 * bytes as they arrive regardless, since a host that would send an endless
 * body is not one whose headers mean anything. */
async function readCappedBody(response: Response, url: string | URL): Promise<Uint8Array> {
  const declared = response.headers.get("content-length");
  if (declared !== null && Number(declared) > MAX_MANIFEST_BYTES) {
    throw manifestTooLarge(url, `it declares ${declared} bytes`);
  }

  // A Response with no body at all (a fake, or a 204) has nothing to stream.
  if (response.body === null) {
    const whole = new Uint8Array(await response.arrayBuffer());
    if (whole.length > MAX_MANIFEST_BYTES) {
      throw manifestTooLarge(url, `it is ${whole.length} bytes`);
    }
    return whole;
  }

  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.length;
    if (total > MAX_MANIFEST_BYTES) {
      await reader.cancel();
      throw manifestTooLarge(url, `it is still streaming past ${MAX_MANIFEST_BYTES} bytes`);
    }
    chunks.push(value);
  }

  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.length;
  }
  return bytes;
}

function manifestTooLarge(url: string | URL, detail: string): GosdManifestFetchError {
  return new GosdManifestFetchError(
    `fetching the injection manifest at ${url} was abandoned because ${detail}, past the ${MAX_MANIFEST_BYTES}-byte limit; a gosd manifest is a few KiB of JSON, so check the URL names the .inject.json sidecar rather than the image itself`,
  );
}

/** Whether url is plain `http:` somewhere a network can see. Loopback is
 * exempt: a dev server on http://localhost has no path for anyone to sit on,
 * and a warning every local run would only teach people to ignore it. */
function isOverPlainHTTP(url: string | URL): boolean {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return false;
  }
  if (parsed.protocol !== "http:") return false;
  const host = parsed.hostname;
  return !(host === "localhost" || host === "[::1]" || host === "::1" || host.startsWith("127."));
}

/** Structurally validates a parsed JSON value as a Manifest: every field is
 * present and the right shape, each placeholder's ranges sum to its size
 * and lie within the image, and no two placeholders' ranges overlap. Throws
 * `GosdManifestInvalidError` naming the offending JSON path on the first
 * problem found. */
export function parseManifest(data: unknown): Manifest {
  const obj = expectRecord(data, "manifest");
  const image = parseImageInfo(expectRecord(obj.image, "manifest.image"));
  const placeholders = expectArray(obj.placeholders, "manifest.placeholders").map((p, i) =>
    parsePlaceholderInfo(p, `manifest.placeholders[${i}]`, image.size),
  );
  // An image with no config tree at all publishes no key rather than an
  // empty array, so a missing one is "nothing to inject", not a malformed
  // manifest.
  const config =
    obj.config === undefined
      ? []
      : expectArray(obj.config, "manifest.config").map((c, i) =>
          parseConfigInfo(c, `manifest.config[${i}]`, image.size),
        );

  const manifest: Manifest = {
    gosd_inject: expectLiteral(obj.gosd_inject, 1, "manifest.gosd_inject"),
    board: expectString(obj.board, "manifest.board"),
    image,
    placeholders,
    config,
  };
  checkNoOverlaps(manifest);
  return manifest;
}

function parseConfigInfo(value: unknown, at: string, imageSize: number): ConfigInfo {
  const obj = expectRecord(value, at);
  const size = expectNonNegativeInt(obj.size, `${at}.size`);
  const ranges = expectArray(obj.ranges, `${at}.ranges`).map((r, i) =>
    parseByteRange(r, `${at}.ranges[${i}]`, imageSize),
  );
  if (ranges.length === 0) {
    throw new GosdManifestInvalidError(`${at}.ranges: must have at least one range`);
  }
  const total = ranges.reduce((sum, r) => sum + r.length, 0);
  if (total !== size) {
    throw new GosdManifestInvalidError(`${at}: ranges sum to ${total} bytes but size is ${size}`);
  }
  return {
    path: expectString(obj.path, `${at}.path`),
    size,
    sha256: expectSha256Hex(obj.sha256, `${at}.sha256`),
    ranges,
    value: expectString(obj.value, `${at}.value`),
  };
}

function parseImageInfo(obj: Record<string, unknown>): ImageInfo {
  return {
    filename: expectString(obj.filename, "manifest.image.filename"),
    size: expectNonNegativeInt(obj.size, "manifest.image.size"),
    sha256: expectSha256Hex(obj.sha256, "manifest.image.sha256"),
  };
}

function parsePlaceholderInfo(value: unknown, at: string, imageSize: number): PlaceholderInfo {
  const obj = expectRecord(value, at);
  const size = expectNonNegativeInt(obj.size, `${at}.size`);
  const ranges = expectArray(obj.ranges, `${at}.ranges`).map((r, i) =>
    parseByteRange(r, `${at}.ranges[${i}]`, imageSize),
  );
  if (ranges.length === 0) {
    throw new GosdManifestInvalidError(`${at}.ranges: must have at least one range`);
  }
  const total = ranges.reduce((sum, r) => sum + r.length, 0);
  if (total !== size) {
    throw new GosdManifestInvalidError(`${at}: ranges sum to ${total} bytes but size is ${size}`);
  }
  return {
    path: expectString(obj.path, `${at}.path`),
    size,
    sha256: expectSha256Hex(obj.sha256, `${at}.sha256`),
    ranges,
  };
}

function parseByteRange(value: unknown, at: string, imageSize: number): ByteRange {
  const obj = expectRecord(value, at);
  const offset = expectNonNegativeInt(obj.offset, `${at}.offset`);
  const length = expectNonNegativeInt(obj.length, `${at}.length`);
  if (offset + length > imageSize) {
    throw new GosdManifestInvalidError(
      `${at}: range [${offset}, ${offset + length}) extends past the image's size of ${imageSize} bytes`,
    );
  }
  return { offset, length };
}

/** Every region's ranges, pooled across the whole manifest, must be
 * disjoint — two regions can't claim the same image byte, so patching one
 * can never partly overwrite another. */
function checkNoOverlaps(manifest: Manifest): void {
  const intervals = injectableRegions(manifest)
    .flatMap((region) =>
      region.ranges.map((r) => ({
        start: r.offset,
        end: r.offset + r.length,
        label: region.label,
      })),
    )
    .sort((a, b) => a.start - b.start);

  for (let i = 1; i < intervals.length; i++) {
    const prev = intervals[i - 1];
    const cur = intervals[i];
    if (prev !== undefined && cur !== undefined && cur.start < prev.end) {
      throw new GosdManifestInvalidError(
        `manifest: ${cur.label}'s range [${cur.start}, ${cur.end}) overlaps ${prev.label}'s range [${prev.start}, ${prev.end})`,
      );
    }
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function describe(value: unknown): string {
  if (value === null) return "null";
  if (Array.isArray(value)) return "an array";
  return typeof value;
}

function expectRecord(value: unknown, at: string): Record<string, unknown> {
  if (!isRecord(value))
    throw new GosdManifestInvalidError(`${at}: expected an object, got ${describe(value)}`);
  return value;
}

function expectArray(value: unknown, at: string): unknown[] {
  if (!Array.isArray(value))
    throw new GosdManifestInvalidError(`${at}: expected an array, got ${describe(value)}`);
  return value;
}

function expectString(value: unknown, at: string): string {
  if (typeof value !== "string")
    throw new GosdManifestInvalidError(`${at}: expected a string, got ${describe(value)}`);
  return value;
}

function expectNonNegativeInt(value: unknown, at: string): number {
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0) {
    throw new GosdManifestInvalidError(
      `${at}: expected a non-negative integer, got ${JSON.stringify(value)}`,
    );
  }
  return value;
}

function expectSha256Hex(value: unknown, at: string): string {
  const s = expectString(value, at);
  if (!/^[0-9a-f]{64}$/.test(s)) {
    throw new GosdManifestInvalidError(
      `${at}: expected a 64-character lowercase hex sha256, got ${JSON.stringify(s)}`,
    );
  }
  return s;
}

function expectLiteral<T extends number>(value: unknown, expected: T, at: string): T {
  if (value !== expected) {
    throw new GosdManifestInvalidError(
      `${at}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(value)}`,
    );
  }
  return expected;
}
