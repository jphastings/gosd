import { describe, expect, it, vi } from "vitest";
import { padContents } from "./content.js";
import { GosdImageFetchError, GosdImagePreconditionError, GosdSaveFailedError } from "./errors.js";
import type { Manifest } from "./manifest.js";
import {
  clampToSafeResumeOffset,
  createFreshDownloadCheckpoint,
  discardResumableDownload,
  listResumableDownloads,
  reconstructPristinePrefix,
  resumeDownload,
} from "./resume.js";
import type { ResumeRecord, ResumeStore } from "./resume-store.js";
import { Sha256 } from "./sha256.js";
import type { SeekableSaveSink } from "./sinks/types.js";

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
): { image: Uint8Array<ArrayBuffer>; manifest: Manifest } {
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
    return { path: spec.path, size, sha256: sha256Hex(content), ranges: spec.ranges };
  });

  const manifest: Manifest = {
    gosd_inject: 1,
    board: "test",
    image: { filename: "fixture.img", size: imageSize, sha256: sha256Hex(image) },
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

function fakeStore(initial: ResumeRecord[] = []): ResumeStore {
  const data = new Map(initial.map((r) => [r.key, r]));
  return {
    async get(key) {
      return data.get(key);
    },
    async put(record) {
      data.set(record.key, record);
    },
    async delete(key) {
      data.delete(key);
    },
    async list() {
      return Array.from(data.values());
    },
  };
}

/** An in-memory stand-in for a persisted FileSystemFileHandle: a growable
 * buffer, `createWritable({keepExistingData})` truncating or not, and a
 * `seek` on the returned writable — exactly the surface
 * openPersistedFsAccessHandle (the real fs-access.ts code, exercised
 * as-is by these tests) needs. */
function fakeFileHandle(initialBytes: Uint8Array = new Uint8Array(0)) {
  let buffer = initialBytes.slice();
  const requestPermission = vi.fn(async () => "granted" as const);

  return {
    requestPermission,
    snapshot: () => buffer.slice(),
    async createWritable(options?: { keepExistingData?: boolean }) {
      if (!options?.keepExistingData) buffer = new Uint8Array(0);
      let cursor = 0;
      const writable = new WritableStream<Uint8Array>({
        write(chunk) {
          const end = cursor + chunk.length;
          if (end > buffer.length) {
            const grown = new Uint8Array(end);
            grown.set(buffer);
            buffer = grown;
          }
          buffer.set(chunk, cursor);
          cursor += chunk.length;
        },
      }) as WritableStream<Uint8Array> & { seek(position: number): Promise<void> };
      writable.seek = async (position: number) => {
        cursor = position;
      };
      return writable;
    },
    async getFile() {
      const snapshot = buffer.slice();
      return {
        size: snapshot.length,
        async arrayBuffer() {
          return snapshot.buffer.slice(snapshot.byteOffset, snapshot.byteOffset + snapshot.length);
        },
      };
    },
  };
}

describe("clampToSafeResumeOffset", () => {
  const { manifest } = buildFixture(1000, [
    { path: "patched", ranges: [{ offset: 100, length: 50 }] },
    { path: "untouched", ranges: [{ offset: 300, length: 50 }] },
  ]);
  const padded = new Map([["patched", new Uint8Array(50)]]);

  it("leaves the offset alone when it's outside every placeholder", () => {
    expect(clampToSafeResumeOffset(manifest, padded, {}, 10)).toBe(10);
    expect(clampToSafeResumeOffset(manifest, padded, {}, 100)).toBe(100);
    expect(clampToSafeResumeOffset(manifest, padded, {}, 150)).toBe(150);
  });

  it("rolls back to the range start when straddling a patched, not-yet-captured placeholder", () => {
    expect(clampToSafeResumeOffset(manifest, padded, {}, 120)).toBe(100);
  });

  it("doesn't roll back once the placeholder's pristine bytes were captured", () => {
    const captured = { patched: new Uint8Array(50) };
    expect(clampToSafeResumeOffset(manifest, padded, captured, 120)).toBe(120);
  });

  it("doesn't roll back for an untouched placeholder even when straddled", () => {
    expect(clampToSafeResumeOffset(manifest, padded, {}, 320)).toBe(320);
  });
});

describe("reconstructPristinePrefix", () => {
  const { image, manifest } = buildFixture(500, [
    { path: "patched", ranges: [{ offset: 100, length: 20 }] },
    { path: "untouched", ranges: [{ offset: 300, length: 10 }] },
  ]);
  const padded = new Map([["patched", new Uint8Array(20).fill(0xaa)]]);
  const onDisk = naiveSplice(image, manifest, padded);

  it("swaps a captured placeholder's on-disk bytes back for its stashed pristine bytes", () => {
    const reconstructed = reconstructPristinePrefix(onDisk.subarray(0, 200), manifest, {
      patched: image.subarray(100, 120),
    });
    expect(reconstructed).toEqual(image.subarray(0, 200));
  });

  it("leaves bytes outside any captured placeholder's range untouched", () => {
    const reconstructed = reconstructPristinePrefix(onDisk.subarray(0, 90), manifest, {});
    expect(reconstructed).toEqual(image.subarray(0, 90));
  });

  it("leaves a placeholder's on-disk bytes as-is when its range extends past the given prefix", () => {
    // "patched" wasn't fully captured but is (incorrectly) claimed captured
    // here; its range [100,120) exceeds the 110-byte prefix, so the
    // reconstruction must not touch it (the resulting mismatch is caught by
    // primeSubstitutionState instead of silently reconstructing wrongly).
    const reconstructed = reconstructPristinePrefix(onDisk.subarray(0, 110), manifest, {
      patched: image.subarray(100, 120),
    });
    expect(reconstructed.subarray(100, 110)).toEqual(onDisk.subarray(100, 110));
  });
});

describe("createFreshDownloadCheckpoint", () => {
  function fixtureSink(): SeekableSaveSink & { setReadExisting(bytes: Uint8Array): void } {
    let readExistingBytes: Uint8Array = new Uint8Array([1, 2, 3]);
    return {
      kind: "fs-access",
      writable: new WritableStream<Uint8Array>(),
      resumeHandle: { fake: "handle" },
      async commit() {},
      async abort() {},
      async readExisting() {
        return readExistingBytes;
      },
      setReadExisting(bytes: Uint8Array) {
        readExistingBytes = bytes;
      },
    };
  }

  it("returns undefined when no store is available and none was given", () => {
    const { manifest } = buildFixture(100, []);
    const checkpoint = createFreshDownloadCheckpoint({
      sink: fixtureSink(),
      manifest,
      imageURL: "https://dl.example.com/app.img",
      filename: "app.img",
      store: undefined,
    });
    // In the Vitest "node" test environment there's no global indexedDB, so
    // the fallback used when no explicit store is given yields undefined.
    expect(checkpoint).toBeUndefined();
  });

  it("persists an initial record immediately, keyed by the manifest's image sha256", async () => {
    const { manifest } = buildFixture(100, []);
    const store = fakeStore();
    const sink = fixtureSink();

    createFreshDownloadCheckpoint({
      sink,
      manifest,
      imageURL: "https://dl.example.com/app.img",
      filename: "app.img",
      store,
    });

    await expect(store.get(manifest.image.sha256)).resolves.toMatchObject({
      key: manifest.image.sha256,
      imageURL: "https://dl.example.com/app.img",
      filename: "app.img",
      bytesWritten: 0,
      pristinePlaceholders: {},
      handle: sink.resumeHandle,
    });
  });

  it("onResponseHeaders and onPlaceholderVerified update the persisted record", async () => {
    const { manifest } = buildFixture(100, []);
    const store = fakeStore();
    const checkpoint = createFreshDownloadCheckpoint({
      sink: fixtureSink(),
      manifest,
      imageURL: "https://dl.example.com/app.img",
      filename: "app.img",
      store,
    })!;

    checkpoint.onResponseHeaders?.({ etag: "abc", lastModified: null });
    checkpoint.onPlaceholderVerified?.("cloud-init.yaml", new Uint8Array([9, 9]));

    await expect(store.get(manifest.image.sha256)).resolves.toMatchObject({
      etag: "abc",
      pristinePlaceholders: { "cloud-init.yaml": new Uint8Array([9, 9]) },
    });
  });

  it("onFinalized(true) deletes the record", async () => {
    const { manifest } = buildFixture(100, []);
    const store = fakeStore();
    const checkpoint = createFreshDownloadCheckpoint({
      sink: fixtureSink(),
      manifest,
      imageURL: "https://dl.example.com/app.img",
      filename: "app.img",
      store,
    })!;

    await checkpoint.onFinalized?.(true);

    await expect(store.get(manifest.image.sha256)).resolves.toBeUndefined();
  });

  it("onFinalized(false) checkpoints bytesWritten from the sink's actual readExisting() length", async () => {
    const { manifest } = buildFixture(100, []);
    const store = fakeStore();
    const sink = fixtureSink();
    sink.setReadExisting(new Uint8Array(42));
    const checkpoint = createFreshDownloadCheckpoint({
      sink,
      manifest,
      imageURL: "https://dl.example.com/app.img",
      filename: "app.img",
      store,
    })!;

    await checkpoint.onFinalized?.(false);

    await expect(store.get(manifest.image.sha256)).resolves.toMatchObject({ bytesWritten: 42 });
  });
});

describe("listResumableDownloads / discardResumableDownload", () => {
  it("lists persisted records, projected to the public shape", async () => {
    const store = fakeStore([
      {
        key: "abc",
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: 1000,
        etag: null,
        lastModified: null,
        bytesWritten: 500,
        pristinePlaceholders: {},
        handle: {},
      },
    ]);

    await expect(listResumableDownloads({ store })).resolves.toEqual([
      {
        key: "abc",
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: 1000,
        bytesWritten: 500,
      },
    ]);
  });

  it("resolves to [] when no store is available", async () => {
    await expect(listResumableDownloads({})).resolves.toEqual([]);
  });

  it("discardResumableDownload deletes the record", async () => {
    const store = fakeStore([
      {
        key: "abc",
        imageURL: "x",
        filename: "x",
        imageSize: 1,
        etag: null,
        lastModified: null,
        bytesWritten: 0,
        pristinePlaceholders: {},
        handle: {},
      },
    ]);

    await discardResumableDownload("abc", { store });

    await expect(store.get("abc")).resolves.toBeUndefined();
  });
});

describe("resumeDownload", () => {
  function fixture() {
    return buildFixture(300, [
      { path: "config.yaml", ranges: [{ offset: 50, length: 20 }] },
      { path: "static.txt", ranges: [{ offset: 200, length: 10 }] },
    ]);
  }

  it("throws GosdSaveFailedError when no record is stored for the key", async () => {
    await expect(resumeDownload({ key: "missing", files: {}, store: fakeStore() })).rejects.toThrow(
      GosdSaveFailedError,
    );
  });

  it("throws GosdImagePreconditionError when the manifest now describes a different image", async () => {
    const { manifest } = fixture();
    const otherManifest: Manifest = {
      ...manifest,
      image: { ...manifest.image, sha256: "f".repeat(64) },
    };
    const store = fakeStore([
      {
        key: otherManifest.image.sha256,
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: manifest.image.size,
        etag: null,
        lastModified: null,
        bytesWritten: 0,
        pristinePlaceholders: {},
        handle: fakeFileHandle(),
      },
    ]);

    await expect(
      resumeDownload({
        key: otherManifest.image.sha256,
        files: {},
        manifest,
        store,
      }),
    ).rejects.toThrow(GosdImagePreconditionError);
  });

  it("re-verifies the partial file, requests a Range with If-Range, and continues to completion", async () => {
    const { image, manifest } = fixture();
    const files = { "config.yaml": new Uint8Array(20).fill(0xaa) };
    const padded = padContents(files, manifest);
    const expected = naiveSplice(image, manifest, padded);

    const splitPoint = 90; // past config.yaml (50-70), before static.txt (200-210)
    const onDiskSoFar = expected.subarray(0, splitPoint);
    const handle = fakeFileHandle(onDiskSoFar);

    const store = fakeStore([
      {
        key: manifest.image.sha256,
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: manifest.image.size,
        etag: '"the-etag"',
        lastModified: null,
        bytesWritten: splitPoint,
        pristinePlaceholders: { "config.yaml": image.subarray(50, 70) },
        handle,
      },
    ]);

    const fetchCalls: { url: string; range: string | null; ifRange: string | null }[] = [];
    const fakeFetch = (async (url: string | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      fetchCalls.push({
        url: String(url),
        range: headers.get("Range"),
        ifRange: headers.get("If-Range"),
      });
      const rest = image.subarray(splitPoint);
      return new Response(rest, {
        status: 206,
        headers: {
          "content-length": String(rest.length),
          "content-range": `bytes ${splitPoint}-${image.length - 1}/${image.length}`,
        },
      });
    }) as typeof fetch;

    const result = await resumeDownload({
      key: manifest.image.sha256,
      files,
      manifest,
      fetch: fakeFetch,
      store,
    });

    expect(result).toEqual({
      savedVia: "fs-access",
      manifest,
      sha256: manifest.image.sha256,
      filename: "app.img",
    });
    expect(fetchCalls).toEqual([
      {
        url: "https://dl.example.com/app.img",
        range: `bytes=${splitPoint}-`,
        ifRange: '"the-etag"',
      },
    ]);
    expect(handle.snapshot()).toEqual(expected);
    await expect(store.get(manifest.image.sha256)).resolves.toBeUndefined();
  });

  it("restarts from scratch, reusing the same file, when the server ignores the Range (200)", async () => {
    const { image, manifest } = fixture();
    const files = { "config.yaml": new Uint8Array(20).fill(0xaa) };
    const padded = padContents(files, manifest);
    const expected = naiveSplice(image, manifest, padded);

    const splitPoint = 90;
    const handle = fakeFileHandle(expected.subarray(0, splitPoint));
    const store = fakeStore([
      {
        key: manifest.image.sha256,
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: manifest.image.size,
        etag: null,
        lastModified: null,
        bytesWritten: splitPoint,
        pristinePlaceholders: { "config.yaml": image.subarray(50, 70) },
        handle,
      },
    ]);

    const fakeFetch = (async () =>
      new Response(image, {
        status: 200,
        headers: { "content-length": String(image.length) },
      })) as typeof fetch;

    const result = await resumeDownload({
      key: manifest.image.sha256,
      files,
      manifest,
      fetch: fakeFetch,
      store,
    });

    expect(result.sha256).toBe(manifest.image.sha256);
    expect(handle.snapshot()).toEqual(expected);
  });

  it("clamps bytesWritten down to what's actually on disk before resuming", async () => {
    const { image, manifest } = fixture();
    const files = { "config.yaml": new Uint8Array(20).fill(0xaa) };
    const padded = padContents(files, manifest);
    const expected = naiveSplice(image, manifest, padded);

    const trueOnDiskLength = 90;
    const handle = fakeFileHandle(expected.subarray(0, trueOnDiskLength));
    const store = fakeStore([
      {
        key: manifest.image.sha256,
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: manifest.image.size,
        etag: null,
        lastModified: null,
        bytesWritten: 140, // stale: claims more than is really on disk
        pristinePlaceholders: { "config.yaml": image.subarray(50, 70) },
        handle,
      },
    ]);

    let seenRange: string | null = null;
    const fakeFetch = (async (_url: string | URL, init?: RequestInit) => {
      seenRange = new Headers(init?.headers).get("Range");
      const rest = image.subarray(trueOnDiskLength);
      return new Response(rest, {
        status: 206,
        headers: { "content-length": String(rest.length) },
      });
    }) as typeof fetch;

    await resumeDownload({ key: manifest.image.sha256, files, manifest, fetch: fakeFetch, store });

    expect(seenRange).toBe(`bytes=${trueOnDiskLength}-`);
  });

  it("throws GosdImageFetchError for a non-200/206 status on the Range request", async () => {
    const { manifest } = fixture();
    const handle = fakeFileHandle(new Uint8Array(90));
    const store = fakeStore([
      {
        key: manifest.image.sha256,
        imageURL: "https://dl.example.com/app.img",
        filename: "app.img",
        imageSize: manifest.image.size,
        etag: null,
        lastModified: null,
        bytesWritten: 90,
        pristinePlaceholders: {},
        handle,
      },
    ]);
    const fakeFetch = (async () => new Response("nope", { status: 500 })) as typeof fetch;

    await expect(
      resumeDownload({ key: manifest.image.sha256, files: {}, manifest, fetch: fakeFetch, store }),
    ).rejects.toThrow(GosdImageFetchError);
  });
});
