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

export interface Manifest {
  gosd_inject: 1;
  board: string;
  image: ImageInfo;
  placeholders: PlaceholderInfo[];
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

  const bytes = new Uint8Array(await response.arrayBuffer());

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
    throw new GosdManifestInvalidError(
      `the injection manifest at ${url} is not valid JSON`,
      { cause },
    );
  }

  return parseManifest(data);
}

/** Structurally validates a parsed JSON value as a Manifest: every field is
 * present and the right shape, each placeholder's ranges sum to its size
 * and lie within the image, and no two placeholders' ranges overlap. Throws
 * `GosdManifestInvalidError` naming the offending JSON path on the first
 * problem found. */
export function parseManifest(data: unknown): Manifest {
  const obj = expectRecord(data, "manifest");
  const image = parseImageInfo(expectRecord(obj.image, "manifest.image"));
  const placeholders = expectArray(
    obj.placeholders,
    "manifest.placeholders",
  ).map((p, i) =>
    parsePlaceholderInfo(p, `manifest.placeholders[${i}]`, image.size),
  );
  checkNoOverlaps(placeholders, "manifest.placeholders");

  return {
    gosd_inject: expectLiteral(obj.gosd_inject, 1, "manifest.gosd_inject"),
    board: expectString(obj.board, "manifest.board"),
    image,
    placeholders,
  };
}

function parseImageInfo(obj: Record<string, unknown>): ImageInfo {
  return {
    filename: expectString(obj.filename, "manifest.image.filename"),
    size: expectNonNegativeInt(obj.size, "manifest.image.size"),
    sha256: expectSha256Hex(obj.sha256, "manifest.image.sha256"),
  };
}

function parsePlaceholderInfo(
  value: unknown,
  at: string,
  imageSize: number,
): PlaceholderInfo {
  const obj = expectRecord(value, at);
  const size = expectNonNegativeInt(obj.size, `${at}.size`);
  const ranges = expectArray(obj.ranges, `${at}.ranges`).map((r, i) =>
    parseByteRange(r, `${at}.ranges[${i}]`, imageSize),
  );
  if (ranges.length === 0) {
    throw new GosdManifestInvalidError(
      `${at}.ranges: must have at least one range`,
    );
  }
  const total = ranges.reduce((sum, r) => sum + r.length, 0);
  if (total !== size) {
    throw new GosdManifestInvalidError(
      `${at}: ranges sum to ${total} bytes but size is ${size}`,
    );
  }
  return {
    path: expectString(obj.path, `${at}.path`),
    size,
    sha256: expectSha256Hex(obj.sha256, `${at}.sha256`),
    ranges,
  };
}

function parseByteRange(
  value: unknown,
  at: string,
  imageSize: number,
): ByteRange {
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

/** Every placeholder's ranges, pooled across the whole manifest, must be
 * disjoint — two placeholders can't claim the same image byte. */
function checkNoOverlaps(placeholders: PlaceholderInfo[], at: string): void {
  const intervals = placeholders
    .flatMap((p) =>
      p.ranges.map((r) => ({
        start: r.offset,
        end: r.offset + r.length,
        path: p.path,
      })),
    )
    .sort((a, b) => a.start - b.start);

  for (let i = 1; i < intervals.length; i++) {
    const prev = intervals[i - 1];
    const cur = intervals[i];
    if (prev !== undefined && cur !== undefined && cur.start < prev.end) {
      throw new GosdManifestInvalidError(
        `${at}: placeholder "${cur.path}"'s range [${cur.start}, ${cur.end}) overlaps placeholder "${prev.path}"'s range [${prev.start}, ${prev.end})`,
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
    throw new GosdManifestInvalidError(
      `${at}: expected an object, got ${describe(value)}`,
    );
  return value;
}

function expectArray(value: unknown, at: string): unknown[] {
  if (!Array.isArray(value))
    throw new GosdManifestInvalidError(
      `${at}: expected an array, got ${describe(value)}`,
    );
  return value;
}

function expectString(value: unknown, at: string): string {
  if (typeof value !== "string")
    throw new GosdManifestInvalidError(
      `${at}: expected a string, got ${describe(value)}`,
    );
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

function expectLiteral<T extends number>(
  value: unknown,
  expected: T,
  at: string,
): T {
  if (value !== expected) {
    throw new GosdManifestInvalidError(
      `${at}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(value)}`,
    );
  }
  return expected;
}
