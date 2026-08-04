import { afterEach, describe, expect, it, vi } from "vitest";
import { Sha256 } from "./sha256.js";
import {
  GosdCancelledError,
  GosdUnknownPlaceholderError,
  withPlaceholders,
} from "./index.js";

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

function fakeFetch(
  image: Uint8Array<ArrayBuffer>,
  manifestJSON: string,
): typeof fetch {
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
        createWritable: async () =>
          new WritableStream({ write() {}, close() {} }),
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
      withPlaceholders(
        "https://dl.example.com/app.img",
        {},
        { fetch: fetchFn },
      ),
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
    expect(result.sha256).toBe(
      (manifest as { image: { sha256: string } }).image.sha256,
    );
  });
});
