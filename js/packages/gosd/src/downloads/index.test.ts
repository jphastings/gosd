import { afterEach, describe, expect, it, vi } from "vitest";
import { Sha256 } from "./sha256.js";
import { GosdCancelledError, GosdUnknownPlaceholderError, withPlaceholders } from "./index.js";
import type { ResumeRecord, ResumeStore } from "./resume-store.js";

function fakeResumeStore(): ResumeStore & { records: Map<string, ResumeRecord> } {
  const records = new Map<string, ResumeRecord>();
  return {
    records,
    async get(key) {
      return records.get(key);
    },
    async put(record) {
      records.set(record.key, record);
    },
    async delete(key) {
      records.delete(key);
    },
    async list() {
      return Array.from(records.values());
    },
  };
}

function fakeFsAccessPicker() {
  let buffer = new Uint8Array(0);
  const showSaveFilePicker = vi.fn(async () => ({
    createWritable: async () =>
      new WritableStream<Uint8Array>({
        write(chunk) {
          const grown = new Uint8Array(buffer.length + chunk.length);
          grown.set(buffer);
          grown.set(chunk, buffer.length);
          buffer = grown;
        },
      }),
    getFile: async () => {
      const snapshot = buffer;
      return { size: snapshot.length, arrayBuffer: async () => snapshot.buffer };
    },
  }));
  return showSaveFilePicker;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

function fixture(): {
  image: Uint8Array<ArrayBuffer>;
  manifestJSON: string;
  manifest: object;
} {
  const image = new Uint8Array(10).fill(7);
  const sha256 = new Sha256().update(image).digestHex();
  const manifest = {
    gosd_inject: 1,
    board: "test",
    image: { filename: "app.img", size: 10, sha256 },
    placeholders: [],
  };
  return { image, manifestJSON: JSON.stringify(manifest), manifest };
}

function fakeFetch(image: Uint8Array<ArrayBuffer>, manifestJSON: string): typeof fetch {
  return (async (url: string | URL | Request) => {
    const href = url instanceof Request ? url.url : String(url);
    if (href.endsWith(".inject.json")) {
      return new Response(manifestJSON, {
        headers: { "content-type": "application/json" },
      });
    }
    return new Response(image, {
      headers: { "content-length": String(image.length) },
    });
  }) as typeof fetch;
}

function stubShowSaveFilePicker(
  impl: (opts: { suggestedName?: string }) => Promise<unknown>,
): ReturnType<typeof vi.fn> {
  const fn = vi.fn(impl);
  vi.stubGlobal("showSaveFilePicker", fn);
  return fn;
}

describe("withPlaceholders: fs-access gesture ordering", () => {
  it("calls showSaveFilePicker before any other await (e.g. before fetching the manifest)", async () => {
    const order: string[] = [];
    const { image, manifestJSON } = fixture();

    stubShowSaveFilePicker(async () => {
      order.push("picker");
      return {
        createWritable: async () => new WritableStream({ write() {}, close() {} }),
      };
    });

    const fetchFn = fakeFetch(image, manifestJSON);
    const trackedFetch = (async (
      url: Parameters<typeof fetch>[0],
      init?: Parameters<typeof fetch>[1],
    ) => {
      order.push(`fetch:${String(url)}`);
      return fetchFn(url, init);
    }) as typeof fetch;

    const result = await withPlaceholders(
      "https://dl.example.com/app.img",
      {},
      { fetch: trackedFetch },
    );

    expect(order[0]).toBe("picker");
    expect(order.slice(1)).toEqual([
      "fetch:https://dl.example.com/app.inject.json",
      "fetch:https://dl.example.com/app.img",
    ]);
    expect(result.savedVia).toBe("fs-access");
  });

  it("throws GosdCancelledError and never fetches anything when the picker is dismissed", async () => {
    let fetchCalled = false;
    stubShowSaveFilePicker(async () => {
      const err = new Error("dismissed");
      err.name = "AbortError";
      throw err;
    });
    const fetchFn = (async () => {
      fetchCalled = true;
      return new Response(null);
    }) as typeof fetch;

    await expect(
      withPlaceholders("https://dl.example.com/app.img", {}, { fetch: fetchFn }),
    ).rejects.toThrow(GosdCancelledError);
    expect(fetchCalled).toBe(false);
  });
});

describe("withPlaceholders: tier forcing and content validation", () => {
  it("saveVia: 'memory' skips the fs-access picker even when available", async () => {
    const { image, manifestJSON } = fixture();
    const picker = stubShowSaveFilePicker(async () => ({
      createWritable: async () => new WritableStream(),
    }));
    // Only `document` is stubbed — Node has no DOM at all, but its global
    // `URL` already implements createObjectURL/revokeObjectURL for real
    // Blobs, and withPlaceholders also needs a real `URL` constructor
    // (deriveManifestURL, deriveFilenameFromURL), so replacing the whole
    // global would break those.
    vi.stubGlobal("document", {
      createElement: () => ({ style: {}, click() {} }),
      body: { appendChild: () => {}, removeChild: () => {} },
    });

    const result = await withPlaceholders(
      "https://dl.example.com/app.img",
      {},
      { fetch: fakeFetch(image, manifestJSON), saveVia: "memory" },
    );

    expect(picker).not.toHaveBeenCalled();
    expect(result.savedVia).toBe("memory");
  });

  it("rejects an unknown placeholder key with GosdUnknownPlaceholderError", async () => {
    const { image, manifestJSON } = fixture();
    await expect(
      withPlaceholders(
        "https://dl.example.com/app.img",
        { "no-such-file": "x" },
        { fetch: fakeFetch(image, manifestJSON), saveVia: "memory" },
      ),
    ).rejects.toThrow(GosdUnknownPlaceholderError);
  });

  it("accepts a pre-fetched manifest via options.manifest, skipping the manifest fetch", async () => {
    vi.stubGlobal("document", {
      createElement: () => ({ style: {}, click() {} }),
      body: { appendChild: () => {}, removeChild: () => {} },
    });
    const { image, manifest } = fixture();
    const fetchCalls: string[] = [];
    const fetchFn = (async (url: Parameters<typeof fetch>[0]) => {
      fetchCalls.push(String(url));
      return new Response(image, {
        headers: { "content-length": String(image.length) },
      });
    }) as typeof fetch;

    const result = await withPlaceholders(
      "https://dl.example.com/app.img",
      {},
      { fetch: fetchFn, saveVia: "memory", manifest: manifest as never },
    );

    expect(fetchCalls).toEqual(["https://dl.example.com/app.img"]);
    expect(result.sha256).toBe((manifest as { image: { sha256: string } }).image.sha256);
  });
});

describe("withPlaceholders: resumable", () => {
  it("persists a resume record while downloading, and deletes it on success, when options.resumable is set", async () => {
    const { image, manifestJSON, manifest } = fixture();
    vi.stubGlobal("showSaveFilePicker", fakeFsAccessPicker());
    const store = fakeResumeStore();
    const sha256 = (manifest as { image: { sha256: string } }).image.sha256;

    const result = await withPlaceholders(
      "https://dl.example.com/app.img",
      {},
      {
        fetch: fakeFetch(image, manifestJSON),
        resumable: true,
        resumeStore: store,
      },
    );

    expect(result.savedVia).toBe("fs-access");
    expect(store.records.has(sha256)).toBe(false);
  });

  it("never touches a resume store when options.resumable is left off (the default)", async () => {
    const { image, manifestJSON } = fixture();
    vi.stubGlobal("showSaveFilePicker", fakeFsAccessPicker());
    const store = fakeResumeStore();
    const put = vi.spyOn(store, "put");

    await withPlaceholders(
      "https://dl.example.com/app.img",
      {},
      { fetch: fakeFetch(image, manifestJSON), resumeStore: store },
    );

    expect(put).not.toHaveBeenCalled();
  });

  it("leaves a checkpoint behind (not deleted) when the download doesn't finish", async () => {
    const { manifestJSON, manifest } = fixture();
    vi.stubGlobal("showSaveFilePicker", fakeFsAccessPicker());
    const store = fakeResumeStore();
    const sha256 = (manifest as { image: { sha256: string } }).image.sha256;

    // The manifest fetch succeeds, but the image fetch fails outright (a
    // network drop before any bytes stream) — a recoverable failure, so
    // the checkpoint created after the manifest is known should survive.
    const flakyFetch = (async (url: string | URL | Request) => {
      const href = url instanceof Request ? url.url : String(url);
      if (href.endsWith(".inject.json")) {
        return new Response(manifestJSON, { headers: { "content-type": "application/json" } });
      }
      throw new Error("network drop");
    }) as typeof fetch;

    await expect(
      withPlaceholders(
        "https://dl.example.com/app.img",
        {},
        { fetch: flakyFetch, resumable: true, resumeStore: store },
      ),
    ).rejects.toThrow();

    expect(store.records.has(sha256)).toBe(true);
  });
});
