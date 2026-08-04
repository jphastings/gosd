import { describe, expect, it } from "vitest";
import {
  GosdImageHashMismatchError,
  GosdImageSizeError,
  GosdPlaceholderNotPristineError,
} from "./errors.js";
import type { Manifest } from "./manifest.js";
import { Sha256 } from "./sha256.js";
import {
  createSubstitutionTransform,
  patchStream,
  type SubstitutionProgress,
} from "./substitute.js";

function sha256Hex(bytes: Uint8Array): string {
  return new Sha256().update(bytes).digestHex();
}

interface PlaceholderSpec {
  path: string;
  ranges: { offset: number; length: number }[];
}

function buildFixture(
  imageSize: number,
  specs: PlaceholderSpec[],
): { image: Uint8Array; manifest: Manifest } {
  const image = new Uint8Array(imageSize);
  for (let i = 0; i < imageSize; i++) image[i] = (i * 2654435761 + 12345) % 256;

  const placeholders = specs.map((spec) => {
    const size = spec.ranges.reduce((sum, r) => sum + r.length, 0);
    const content = new Uint8Array(size);
    let consumed = 0;
    for (const r of spec.ranges) {
      content.set(image.subarray(r.offset, r.offset + r.length), consumed);
      consumed += r.length;
    }
    return {
      path: spec.path,
      size,
      sha256: sha256Hex(content),
      ranges: spec.ranges,
    };
  });

  const manifest: Manifest = {
    gosd_inject: 1,
    board: "test",
    image: {
      filename: "fixture.img",
      size: imageSize,
      sha256: sha256Hex(image),
    },
    placeholders,
  };

  return { image, manifest };
}

function naiveSplice(
  image: Uint8Array,
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
): Uint8Array {
  const out = Uint8Array.from(image);
  for (const p of manifest.placeholders) {
    const replacement = padded.get(p.path);
    if (!replacement) continue;
    let consumed = 0;
    for (const r of p.ranges) {
      out.set(replacement.subarray(consumed, consumed + r.length), r.offset);
      consumed += r.length;
    }
  }
  return out;
}

function chunksOfSizes(data: Uint8Array, sizes: number[]): Uint8Array[] {
  const out: Uint8Array[] = [];
  let offset = 0;
  let i = 0;
  while (offset < data.length) {
    const size = sizes[i % sizes.length] ?? data.length - offset;
    const end = Math.min(offset + size, data.length);
    out.push(data.subarray(offset, end));
    offset = end;
    i++;
  }
  return out;
}

function readableFrom(chunks: Uint8Array[]): ReadableStream<Uint8Array> {
  return new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(c);
      controller.close();
    },
  });
}

async function collect(readable: ReadableStream<Uint8Array>): Promise<Uint8Array> {
  const parts: Uint8Array[] = [];
  const reader = readable.getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    parts.push(value);
  }
  const total = parts.reduce((sum, p) => sum + p.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const p of parts) {
    out.set(p, offset);
    offset += p.length;
  }
  return out;
}

describe("createSubstitutionTransform: chunk-boundary matrix", () => {
  const { image, manifest } = buildFixture(2000, [
    { path: "a", ranges: [{ offset: 100, length: 50 }] },
    {
      path: "b",
      ranges: [
        { offset: 500, length: 30 },
        { offset: 600, length: 20 },
      ],
    },
    { path: "c", ranges: [{ offset: 1000, length: 1 }] },
    { path: "d", ranges: [{ offset: 1998, length: 2 }] }, // touches the very end
  ]);

  const padded = new Map([
    ["a", new Uint8Array(50).fill(0xaa)],
    ["b", new Uint8Array(50).fill(0xbb)],
    ["d", new Uint8Array(2).fill(0xdd)],
    // "c" deliberately left untouched
  ]);

  const expected = naiveSplice(image, manifest, padded);

  const framings: Record<string, number[]> = {
    "a single giant chunk": [2000],
    "1-byte chunks": [1],
    "chunks aligned exactly to range boundaries": [100, 50, 350, 30, 70, 20, 380, 1, 997, 2],
    "chunks unaligned to any range": [37],
    "a few large chunks, several ranges each": [900, 900, 200],
    "a seeded pseudo-random partition": [7, 13, 29, 3, 101, 2, 17, 41],
  };

  for (const [name, sizes] of Object.entries(framings)) {
    it(`produces byte-identical output to a naive splice: ${name}`, async () => {
      const output = await collect(
        patchStream(readableFrom(chunksOfSizes(image, sizes)), manifest, padded),
      );
      expect(output).toEqual(expected);
    });
  }

  it("leaves the untouched placeholder's bytes starting with its original content", async () => {
    const output = await collect(
      patchStream(readableFrom(chunksOfSizes(image, [64])), manifest, padded),
    );
    expect(output.subarray(1000, 1001)).toEqual(image.subarray(1000, 1001));
  });

  it("reports progress using the manifest's declared size as bytesTotal", async () => {
    const events: SubstitutionProgress[] = [];
    await collect(
      patchStream(readableFrom(chunksOfSizes(image, [333])), manifest, padded, {
        onProgress: (p) => events.push(p),
      }),
    );
    expect(events.every((e) => e.bytesTotal === 2000)).toBe(true);
    expect(events.at(-1)).toEqual({ bytesProcessed: 2000, bytesTotal: 2000 });
  });
});

describe("createSubstitutionTransform: corruption", () => {
  it("throws GosdPlaceholderNotPristineError when a patched placeholder's bytes are tampered", async () => {
    const { image, manifest } = buildFixture(500, [
      { path: "a", ranges: [{ offset: 100, length: 50 }] },
    ]);
    const tampered = Uint8Array.from(image);
    tampered[110] ^= 0xff;

    await expect(
      collect(patchStream(readableFrom([tampered]), manifest, new Map())),
    ).rejects.toThrow(GosdPlaceholderNotPristineError);
  });

  it("throws GosdPlaceholderNotPristineError for a tampered untouched placeholder too", async () => {
    const { image, manifest } = buildFixture(500, [
      { path: "a", ranges: [{ offset: 100, length: 50 }] },
    ]);
    const tampered = Uint8Array.from(image);
    tampered[149] ^= 0xff;

    await expect(
      collect(patchStream(readableFrom([tampered]), manifest, new Map())),
    ).rejects.toThrow(GosdPlaceholderNotPristineError);
  });

  it("throws GosdImageHashMismatchError (not a placeholder error) when corruption is outside every placeholder", async () => {
    const { image, manifest } = buildFixture(500, [
      { path: "a", ranges: [{ offset: 100, length: 50 }] },
    ]);
    const tampered = Uint8Array.from(image);
    tampered[0] ^= 0xff;

    await expect(
      collect(patchStream(readableFrom([tampered]), manifest, new Map())),
    ).rejects.toThrow(GosdImageHashMismatchError);
  });

  it("detects a tampered placeholder at the earliest possible chunk, emitting nothing after it", async () => {
    const { image, manifest } = buildFixture(500, [
      { path: "a", ranges: [{ offset: 100, length: 50 }] },
    ]);
    const tampered = Uint8Array.from(image);
    tampered[110] ^= 0xff;

    const reader = patchStream(
      readableFrom(chunksOfSizes(tampered, [1])),
      manifest,
      new Map(),
    ).getReader();
    let seen = 0;
    await expect(
      (async () => {
        for (;;) {
          const { done, value } = await reader.read();
          if (done) return;
          seen += value.length;
        }
      })(),
    ).rejects.toThrow(GosdPlaceholderNotPristineError);
    // The placeholder ends at offset 150; nothing past that should ever
    // have been enqueued once its hash failed.
    expect(seen).toBeLessThanOrEqual(150);
  });
});

describe("createSubstitutionTransform: stream length", () => {
  it("throws GosdImageSizeError on a short stream", async () => {
    const { image, manifest } = buildFixture(500, []);
    await expect(
      collect(patchStream(readableFrom([image.subarray(0, 400)]), manifest, new Map())),
    ).rejects.toThrow(GosdImageSizeError);
  });

  it("throws GosdImageSizeError on an overlong stream, before the overflow bytes reach the sink", async () => {
    const { image, manifest } = buildFixture(500, []);
    const overlong = new Uint8Array(600);
    overlong.set(image);

    const reader = patchStream(readableFrom([overlong]), manifest, new Map()).getReader();
    await expect(reader.read()).rejects.toThrow(GosdImageSizeError);
  });
});

describe("createSubstitutionTransform: degenerate manifests", () => {
  it("handles a manifest with no placeholders at all", async () => {
    const { image, manifest } = buildFixture(200, []);
    const output = await collect(
      patchStream(readableFrom(chunksOfSizes(image, [17])), manifest, new Map()),
    );
    expect(output).toEqual(image);
  });

  it("verifies a zero-size placeholder immediately, without waiting for any bytes", async () => {
    const { manifest } = buildFixture(200, [{ path: "empty", ranges: [] }]);
    manifest.placeholders[0]!.size = 0;
    manifest.placeholders[0]!.sha256 = sha256Hex(new Uint8Array(0));
    manifest.placeholders[0]!.ranges = [];

    const transform = createSubstitutionTransform(manifest, new Map());
    expect(transform).toBeInstanceOf(TransformStream);
  });
});
