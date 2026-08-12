// Cross-implementation proof: everything internal/image.Write and
// internal/inject.Render/WriteManifest (Go) actually produce round-trips
// correctly through this package's parseManifest/padContents/runDownload
// (TypeScript) — not just against hand-rolled JS fixtures like
// substitute.test.ts's, which could silently drift from the real contract.
// `npm run genfixture` (a thin wrapper around
// `go run ../../../internal/cmd/injectfixture`) must have already written
// ./fixture/fixture.img and ./fixture/fixture.inject.json before this runs;
// `npm run test:integration` does that for you.

import { createReadStream, readFileSync } from "node:fs";
import path from "node:path";
import { Readable } from "node:stream";
import { fileURLToPath } from "node:url";
import { beforeAll, describe, expect, it } from "vitest";
import {
  deriveManifestURL,
  GosdContentTooLargeError,
  GosdImageHashMismatchError,
  GosdPlaceholderNotPristineError,
  padAll,
  padContents,
  parseManifest,
  runDownload,
  type Manifest,
} from "../src/downloads/index.js";
import type { SaveSink } from "../src/downloads/sinks/types.js";

const fixtureDir = fileURLToPath(new URL("./fixture", import.meta.url));
const imgPath = path.join(fixtureDir, "fixture.img");
const manifestPath = path.join(fixtureDir, "fixture.inject.json");

function collectingSink(): SaveSink & { bytes(): Uint8Array } {
  const chunks: Uint8Array[] = [];
  const writable = new WritableStream<Uint8Array>({
    write(chunk) {
      chunks.push(chunk);
    },
  });
  return {
    kind: "memory",
    writable,
    async commit() {},
    async abort() {
      chunks.length = 0;
    },
    bytes() {
      const total = chunks.reduce((sum, c) => sum + c.length, 0);
      const out = new Uint8Array(total);
      let offset = 0;
      for (const c of chunks) {
        out.set(c, offset);
        offset += c.length;
      }
      return out;
    },
  };
}

describe("cross-implementation integration: gosd build --placeholder round trip", () => {
  let manifest: Manifest;
  let pristine: Uint8Array;

  beforeAll(() => {
    manifest = parseManifest(JSON.parse(readFileSync(manifestPath, "utf8")));
    // Wrapped in a plain Uint8Array: readFileSync returns a Buffer, whose
    // different prototype makes vitest's toEqual see it as unequal to a
    // same-valued plain Uint8Array (what the sink collects) even though
    // Buffer is itself a Uint8Array subclass.
    pristine = new Uint8Array(readFileSync(imgPath));
  });

  it("derives the same manifest basename as the sidecar gosd actually wrote", () => {
    const derived = deriveManifestURL("https://dl.example.com/fixture.img");
    expect(path.basename(derived.pathname)).toBe(path.basename(manifestPath));
  });

  it("streams, verifies, and patches a real gosd-built image", async () => {
    const padded = padContents({ "config.yaml": "hello from the integration test\n" }, manifest);
    const sink = collectingSink();

    const result = await runDownload({
      manifest,
      padded,
      fetchImage: async () =>
        new Response(
          Readable.toWeb(createReadStream(imgPath)) as unknown as ReadableStream<Uint8Array>,
          {
            headers: {
              "content-type": "application/octet-stream",
              "content-length": String(manifest.image.size),
              etag: manifest.image.sha256,
            },
          },
        ),
      sink,
    });

    expect(result.sha256).toBe(manifest.image.sha256);
    const output = sink.bytes();
    expect(output.length).toBe(manifest.image.size);

    // Every patched placeholder's ranges hold exactly the padded content;
    // the untouched one's ranges still read as a pristine placeholder.
    for (const placeholder of manifest.placeholders) {
      const replacement = padded.get(placeholder.path);
      let consumed = 0;
      for (const range of placeholder.ranges) {
        const slice = output.subarray(range.offset, range.offset + range.length);
        if (replacement) {
          expect(slice).toEqual(replacement.subarray(consumed, consumed + range.length));
        } else {
          expect(new TextDecoder().decode(slice.subarray(0, 40))).toMatch(/^# GOSD-PLACEHOLDER/);
          expect(slice).toEqual(pristine.subarray(range.offset, range.offset + range.length));
        }
        consumed += range.length;
      }
    }

    // Every byte outside a patched range is byte-for-byte identical to the
    // pristine fixture — build the expected output by patching a copy of
    // the pristine bytes directly, and compare the whole buffer at once.
    const expected = Uint8Array.from(pristine);
    for (const [placeholderPath, replacement] of padded) {
      const placeholder = manifest.placeholders.find((p) => p.path === placeholderPath);
      if (!placeholder)
        throw new Error(`test setup error: no placeholder named ${placeholderPath}`);
      let consumed = 0;
      for (const range of placeholder.ranges) {
        expected.set(replacement.subarray(consumed, consumed + range.length), range.offset);
        consumed += range.length;
      }
    }
    // A plain `toEqual` deep-compares this element by element as a generic
    // object, which is minutes-slow at 25MB; Buffer.compare is the native,
    // fast equivalent for two byte buffers of the same length.
    expect(output.length).toBe(expected.length);
    expect(Buffer.compare(output, expected)).toBe(0);
  });

  it("rejects a copy corrupted inside a placeholder's range with GosdPlaceholderNotPristineError", async () => {
    const placeholder = manifest.placeholders.find((p) => p.path === "config.yaml");
    if (!placeholder) throw new Error("test setup error: fixture has no config.yaml placeholder");
    const corrupted = Uint8Array.from(pristine);
    corrupted[placeholder.ranges[0]!.offset + 1] ^= 0xff;

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () =>
          new Response(corrupted, {
            headers: { "content-length": String(manifest.image.size) },
          }),
        sink: collectingSink(),
      }),
    ).rejects.toThrow(GosdPlaceholderNotPristineError);
  });

  it("rejects a copy corrupted outside every placeholder's range with GosdImageHashMismatchError", async () => {
    const corrupted = Uint8Array.from(pristine);
    corrupted[0] ^= 0xff; // byte 0 is deep in the MBR/boot sector, nowhere near a placeholder's content

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () =>
          new Response(corrupted, {
            headers: { "content-length": String(manifest.image.size) },
          }),
        sink: collectingSink(),
      }),
    ).rejects.toThrow(GosdImageHashMismatchError);
  });
});

// The reserved gosd.toml travels a different manifest key than a placeholder
// and carries its own pristine text, so it gets its own round trip against
// the real Go-built fixture.
describe("cross-implementation integration: gosd build --config-placeholder round trip", () => {
  let manifest: Manifest;
  let pristine: Uint8Array;

  beforeAll(() => {
    manifest = parseManifest(JSON.parse(readFileSync(manifestPath, "utf8")));
    pristine = new Uint8Array(readFileSync(imgPath));
  });

  it("publishes the gosd.toml gosd actually wrote, byte for byte", () => {
    if (!manifest.config) throw new Error("fixture manifest has no config region; regenerate it");
    const onCard = readRegion(pristine, manifest.config.ranges);
    expect(new TextDecoder().decode(onCard)).toBe(manifest.config.pristine);
    expect(manifest.config.pristine).toContain('API_URL = "https://example.invalid"');
  });

  it("splices an edit of that text back in, leaving the rest of the image alone", async () => {
    if (!manifest.config) throw new Error("fixture manifest has no config region; regenerate it");
    const padded = padAll(
      {},
      (current) =>
        current.replace(
          '# [ingress.cloudflared]\n# token = "paste-your-tunnel-token-here"\n',
          '[ingress.cloudflared]\ntoken = "injected-token"\n',
        ),
      manifest,
    );
    const sink = collectingSink();

    await runDownload({
      manifest,
      padded,
      fetchImage: async () =>
        new Response(
          Readable.toWeb(createReadStream(imgPath)) as unknown as ReadableStream<Uint8Array>,
          {
            headers: {
              "content-type": "application/octet-stream",
              "content-length": String(manifest.image.size),
              etag: manifest.image.sha256,
            },
          },
        ),
      sink,
    });

    const output = sink.bytes();
    const text = new TextDecoder().decode(readRegion(output, manifest.config.ranges));
    expect(text).toContain('token = "injected-token"');
    expect(text).toContain('API_URL = "https://example.invalid"');
    expect(text).toContain("# These are the settings for this device.");

    const expected = Uint8Array.from(pristine);
    const replacement = padded.get(manifest.config.path);
    if (!replacement) throw new Error("test setup error: no padded content for the config region");
    let consumed = 0;
    for (const range of manifest.config.ranges) {
      expected.set(replacement.subarray(consumed, consumed + range.length), range.offset);
      consumed += range.length;
    }
    expect(Buffer.compare(output, expected)).toBe(0);
  });

  it("refuses a config too long for the reserved region, before anything downloads", () => {
    expect(() => padAll({}, "x".repeat(1_000_000), manifest)).toThrow(GosdContentTooLargeError);
  });
});

/** Concatenates an image's bytes at `ranges`, in order. */
function readRegion(image: Uint8Array, ranges: { offset: number; length: number }[]): Uint8Array {
  const total = ranges.reduce((sum, r) => sum + r.length, 0);
  const out = new Uint8Array(total);
  let consumed = 0;
  for (const r of ranges) {
    out.set(image.subarray(r.offset, r.offset + r.length), consumed);
    consumed += r.length;
  }
  return out;
}
