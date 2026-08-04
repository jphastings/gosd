import { describe, expect, it, vi } from "vitest";
import {
  GosdImageFetchError,
  GosdImageHashMismatchError,
  GosdImagePreconditionError,
} from "./errors.js";
import type { Manifest } from "./manifest.js";
import { runDownload } from "./run.js";
import { Sha256 } from "./sha256.js";
import type { SaveSink } from "./sinks/types.js";

function sha256Hex(bytes: Uint8Array): string {
  return new Sha256().update(bytes).digestHex();
}

function fixture(): { image: Uint8Array<ArrayBuffer>; manifest: Manifest } {
  const image = new Uint8Array(200);
  for (let i = 0; i < image.length; i++) image[i] = i % 256;
  const manifest: Manifest = {
    gosd_inject: 1,
    board: "test",
    image: {
      filename: "app.img",
      size: image.length,
      sha256: sha256Hex(image),
    },
    placeholders: [],
  };
  return { image, manifest };
}

function fakeSink(): SaveSink & {
  writes: Uint8Array[];
  committed: boolean;
  aborts: unknown[];
} {
  const state = {
    writes: [] as Uint8Array[],
    committed: false,
    aborts: [] as unknown[],
  };
  const writable = new WritableStream<Uint8Array>({
    write(chunk) {
      state.writes.push(chunk);
    },
  });
  return {
    kind: "memory",
    writable,
    writes: state.writes,
    get committed() {
      return state.committed;
    },
    aborts: state.aborts,
    async commit() {
      state.committed = true;
    },
    async abort(reason) {
      state.aborts.push(reason);
    },
  } as SaveSink & {
    writes: Uint8Array[];
    committed: boolean;
    aborts: unknown[];
  };
}

describe("runDownload", () => {
  it("fetches, verifies, pipes into the sink, and commits only after the pipe finishes", async () => {
    const { image, manifest } = fixture();
    const sink = fakeSink();

    const result = await runDownload({
      manifest,
      padded: new Map(),
      fetchImage: async () =>
        new Response(image, {
          headers: { "content-length": String(image.length) },
        }),
      sink,
    });

    expect(result.sha256).toBe(manifest.image.sha256);
    expect(sink.committed).toBe(true);
    expect(sink.aborts).toHaveLength(0);
    const written = new Uint8Array(
      sink.writes.reduce((sum, c) => sum + c.length, 0),
    );
    let offset = 0;
    for (const c of sink.writes) {
      written.set(c, offset);
      offset += c.length;
    }
    expect(written).toEqual(image);
  });

  it("aborts the sink and rethrows when fetchImage itself rejects, without committing", async () => {
    const { manifest } = fixture();
    const sink = fakeSink();
    const boom = new Error("network down");

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () => {
          throw boom;
        },
        sink,
      }),
    ).rejects.toThrow(GosdImageFetchError);

    expect(sink.committed).toBe(false);
    expect(sink.aborts).toHaveLength(1);
  });

  it("aborts the sink and rethrows on a non-ok response, without ever writing", async () => {
    const { manifest } = fixture();
    const sink = fakeSink();

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () => new Response("nope", { status: 500 }),
        sink,
      }),
    ).rejects.toThrow(GosdImageFetchError);

    expect(sink.writes).toHaveLength(0);
    expect(sink.committed).toBe(false);
    expect(sink.aborts).toHaveLength(1);
  });

  it("aborts the sink and rethrows on a failed precondition, without ever writing", async () => {
    const { image, manifest } = fixture();
    const sink = fakeSink();

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () =>
          new Response(image, { headers: { "content-length": "1" } }),
        sink,
      }),
    ).rejects.toThrow(GosdImagePreconditionError);

    expect(sink.writes).toHaveLength(0);
    expect(sink.committed).toBe(false);
    expect(sink.aborts).toHaveLength(1);
  });

  it("a matching ETag does not skip the full streamed hash: a corrupt body with a correct ETag still fails", async () => {
    const { image, manifest } = fixture();
    const corrupt = Uint8Array.from(image);
    corrupt[5] ^= 0xff;
    const sink = fakeSink();

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () =>
          new Response(corrupt, {
            headers: {
              etag: manifest.image.sha256,
              "content-length": String(corrupt.length),
            },
          }),
        sink,
      }),
    ).rejects.toThrow(GosdImageHashMismatchError);

    expect(sink.committed).toBe(false);
    expect(sink.aborts).toHaveLength(1);
  });

  it("propagates AbortSignal cancellation to the sink", async () => {
    const { image, manifest } = fixture();
    const sink = fakeSink();
    const controller = new AbortController();
    controller.abort(new Error("user cancelled"));

    await expect(
      runDownload({
        manifest,
        padded: new Map(),
        fetchImage: async () =>
          new Response(image, {
            headers: { "content-length": String(image.length) },
          }),
        sink,
        signal: controller.signal,
      }),
    ).rejects.toThrow();

    expect(sink.committed).toBe(false);
    expect(sink.aborts).toHaveLength(1);
  });

  it("reports progress via onProgress as bytes are patched", async () => {
    const { image, manifest } = fixture();
    const sink = fakeSink();
    const onProgress = vi.fn();

    await runDownload({
      manifest,
      padded: new Map(),
      fetchImage: async () =>
        new Response(image, {
          headers: { "content-length": String(image.length) },
        }),
      sink,
      onProgress,
    });

    expect(onProgress).toHaveBeenCalled();
    const last = onProgress.mock.calls.at(-1)?.[0];
    expect(last).toEqual({
      bytesProcessed: image.length,
      bytesTotal: image.length,
    });
  });
});
