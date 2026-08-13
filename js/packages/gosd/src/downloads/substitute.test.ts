import { describe, expect, it, vi } from "vitest";
import {
  GosdImageHashMismatchError,
  GosdImageSizeError,
  GosdPlaceholderNotPristineError,
} from "./errors.js";
import { configRegionKey, type Manifest } from "./manifest.js";
import { Sha256 } from "./sha256.js";
import {
  createSubstitutionTransform,
  patchStream,
  primeSubstitutionState,
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
    config: [],
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

describe("createSubstitutionTransform: onPlaceholderVerified", () => {
  const { image, manifest } = buildFixture(500, [
    { path: "patched", ranges: [{ offset: 50, length: 20 }] },
    { path: "untouched", ranges: [{ offset: 200, length: 10 }] },
  ]);
  const padded = new Map([["patched", new Uint8Array(20).fill(0xaa)]]);

  it("fires with the pristine bytes for a patched placeholder, but never for an untouched one", async () => {
    const events: Array<{ path: string; pristine: Uint8Array }> = [];
    await collect(
      patchStream(readableFrom(chunksOfSizes(image, [33])), manifest, padded, {
        onPlaceholderVerified: (path, pristine) =>
          events.push({ path, pristine: pristine.slice() }),
      }),
    );

    expect(events).toHaveLength(1);
    expect(events[0]?.path).toBe("patched");
    expect(events[0]?.pristine).toEqual(image.subarray(50, 70));
  });
});

describe("primeSubstitutionState + resumeFrom: continuing a download across a session", () => {
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
  ]);
  const padded = new Map([
    ["a", new Uint8Array(50).fill(0xaa)],
    ["b", new Uint8Array(50).fill(0xbb)],
    // "c" deliberately left untouched
  ]);
  const expected = naiveSplice(image, manifest, padded);

  const splitPoints: Record<string, number> = {
    "before any placeholder": 10,
    "mid-way through a patched placeholder (a, 100-150)": 120,
    "between two ranges of the same fragmented placeholder (b)": 550,
    "exactly on the untouched placeholder's boundary (c, 1000-1001)": 1000,
    "just before the end": 1999,
  };

  for (const [name, splitPoint] of Object.entries(splitPoints)) {
    it(`resumes correctly when split ${name} (offset ${splitPoint})`, async () => {
      const resumeState = primeSubstitutionState(manifest, padded, image.subarray(0, splitPoint));

      const rest = image.subarray(splitPoint);
      const output = await collect(
        patchStream(readableFrom(chunksOfSizes(rest, [37])), manifest, padded, {}, resumeState),
      );

      expect(output).toEqual(expected.subarray(splitPoint));
    });
  }

  it("reports onPlaceholderVerified only for placeholders finishing after the resume point", async () => {
    const splitPoint = 150; // placeholder "a" (100-150) already fully captured by this point
    const resumeState = primeSubstitutionState(manifest, padded, image.subarray(0, splitPoint));

    const onPlaceholderVerified = vi.fn();
    await collect(
      patchStream(
        readableFrom(chunksOfSizes(image.subarray(splitPoint), [40])),
        manifest,
        padded,
        { onPlaceholderVerified },
        resumeState,
      ),
    );

    expect(onPlaceholderVerified).toHaveBeenCalledTimes(1);
    expect(onPlaceholderVerified).toHaveBeenCalledWith("b", expect.any(Uint8Array));
  });

  it("primeSubstitutionState throws GosdPlaceholderNotPristineError when the prefix was reconstructed wrong", () => {
    const tamperedPrefix = Uint8Array.from(image.subarray(0, 200));
    tamperedPrefix[110] ^= 0xff; // inside placeholder "a"'s range

    expect(() => primeSubstitutionState(manifest, padded, tamperedPrefix)).toThrow(
      GosdPlaceholderNotPristineError,
    );
  });

  it("primeSubstitutionState on an empty prefix behaves like starting fresh", async () => {
    const resumeState = primeSubstitutionState(manifest, padded, new Uint8Array(0));
    const output = await collect(
      patchStream(readableFrom(chunksOfSizes(image, [51])), manifest, padded, {}, resumeState),
    );
    expect(output).toEqual(expected);
  });
});

// A config tree setting is a value file of its own, not a placeholder, and
// travels a different manifest key to get here — these pin that it gets the
// same substitution and pristine-verification behaviour a placeholder does.
describe("a config tree setting", () => {
  function fixtureWithSetting(): { image: Uint8Array; manifest: Manifest } {
    const { image, manifest } = buildFixture(4096, [
      { path: "app.yaml", ranges: [{ offset: 0, length: 64 }] },
    ]);
    const ranges = [{ offset: 1024, length: 128 }];
    const pristine = image.subarray(1024, 1024 + 128);
    return {
      image,
      manifest: {
        ...manifest,
        config: [{ path: "wifi/ssid", size: 128, sha256: sha256Hex(pristine), ranges, value: "" }],
      },
    };
  }

  it("substitutes its bytes and leaves the rest of the image alone", async () => {
    const { image, manifest } = fixtureWithSetting();
    const replacement = new Uint8Array(128).fill(0x41);

    const out = await collect(
      patchStream(
        readableFrom([image]),
        manifest,
        new Map([[configRegionKey("wifi/ssid"), replacement]]),
      ),
    );

    expect(out.subarray(1024, 1024 + 128)).toEqual(replacement);
    expect(out.subarray(0, 1024)).toEqual(image.subarray(0, 1024));
    expect(out.subarray(1152)).toEqual(image.subarray(1152));
  });

  it("verifies it is pristine first, naming it in a way a caller can act on", async () => {
    const { image, manifest } = fixtureWithSetting();
    const tampered: Manifest = {
      ...manifest,
      config: [{ ...manifest.config[0]!, sha256: "b".repeat(64) }],
    };

    await expect(
      collect(
        patchStream(
          readableFrom([image]),
          tampered,
          new Map([[configRegionKey("wifi/ssid"), new Uint8Array(128)]]),
        ),
      ),
    ).rejects.toThrow(/setting "wifi\/ssid" is not pristine/);
  });
});
