import { describe, expect, it } from "vitest";
import {
  GosdManifestFetchError,
  GosdManifestHashMismatchError,
  GosdManifestInvalidError,
} from "./errors.js";
import { deriveManifestURL, fetchManifest, parseManifest } from "./manifest.js";
import { Sha256 } from "./sha256.js";

describe("deriveManifestURL", () => {
  it.each([
    ["https://dl.example.com/app-rock-4se.img", "https://dl.example.com/app-rock-4se.inject.json"],
    ["https://dl.example.com/archive.img.gz", "https://dl.example.com/archive.img.inject.json"],
    ["https://dl.example.com/dir/.img", "https://dl.example.com/dir/.inject.json"],
    ["https://dl.example.com/no-extension", "https://dl.example.com/no-extension.inject.json"],
    ["https://dl.example.com/a.b.c", "https://dl.example.com/a.b.inject.json"],
  ])("derives %s -> %s", (input, expected) => {
    expect(deriveManifestURL(input).toString()).toBe(expected);
  });

  it("preserves the query string and drops the fragment", () => {
    const url = deriveManifestURL("https://dl.example.com/app.img?token=abc#section");
    expect(url.toString()).toBe("https://dl.example.com/app.inject.json?token=abc");
  });
});

function validManifest() {
  return {
    gosd_inject: 1,
    board: "pi-zero-2w",
    image: { filename: "app.img", size: 1024, sha256: "a".repeat(64) },
    placeholders: [
      {
        path: "config.yaml",
        size: 100,
        sha256: "b".repeat(64),
        ranges: [{ offset: 200, length: 100 }],
      },
      {
        path: "net.cfg",
        size: 50,
        sha256: "c".repeat(64),
        ranges: [
          { offset: 400, length: 30 },
          { offset: 500, length: 20 },
        ],
      },
    ],
  };
}

describe("parseManifest", () => {
  it("accepts a well-formed manifest", () => {
    const manifest = parseManifest(validManifest());
    expect(manifest.placeholders).toHaveLength(2);
    expect(manifest.image.size).toBe(1024);
  });

  it("rejects a non-object", () => {
    expect(() => parseManifest(null)).toThrow(GosdManifestInvalidError);
    expect(() => parseManifest(null)).toThrow(/^manifest: expected an object/);
  });

  it("rejects the wrong gosd_inject literal, naming the path", () => {
    const m = validManifest();
    (m as unknown as { gosd_inject: number }).gosd_inject = 2;
    expect(() => parseManifest(m)).toThrow(/manifest\.gosd_inject/);
  });

  it("rejects a non-string board, naming the path", () => {
    const m = validManifest();
    (m as unknown as { board: number }).board = 42;
    expect(() => parseManifest(m)).toThrow(/manifest\.board/);
  });

  it("rejects a malformed image.sha256, naming the path", () => {
    const m = validManifest();
    m.image.sha256 = "not-hex";
    expect(() => parseManifest(m)).toThrow(/manifest\.image\.sha256/);
  });

  it("rejects a negative image.size, naming the path", () => {
    const m = validManifest();
    m.image.size = -1;
    expect(() => parseManifest(m)).toThrow(/manifest\.image\.size/);
  });

  it("rejects placeholders whose ranges don't sum to size", () => {
    const m = validManifest();
    m.placeholders[0]!.size = 999;
    expect(() => parseManifest(m)).toThrow(/manifest\.placeholders\[0\]: ranges sum to/);
  });

  it("rejects a placeholder with zero ranges", () => {
    const m = validManifest();
    m.placeholders[0]!.ranges = [];
    expect(() => parseManifest(m)).toThrow(
      /manifest\.placeholders\[0\]\.ranges: must have at least one range/,
    );
  });

  it("rejects a range extending past image.size", () => {
    const m = validManifest();
    m.placeholders[0]!.ranges = [{ offset: 1000, length: 100 }];
    m.placeholders[0]!.size = 100;
    expect(() => parseManifest(m)).toThrow(
      /manifest\.placeholders\[0\]\.ranges\[0\]: range \[1000, 1100\) extends past/,
    );
  });

  it("rejects two placeholders whose ranges overlap", () => {
    const m = validManifest();
    m.placeholders[1]!.ranges = [{ offset: 250, length: 50 }];
    m.placeholders[1]!.size = 50;
    expect(() => parseManifest(m)).toThrow(/overlaps placeholder "config.yaml"/);
  });

  it("rejects a non-array placeholders field", () => {
    const m = validManifest();
    (m as unknown as { placeholders: unknown }).placeholders = {};
    expect(() => parseManifest(m)).toThrow(/manifest\.placeholders: expected an array/);
  });
});

describe("fetchManifest", () => {
  function response(body: unknown, init?: ResponseInit): Response {
    return new Response(typeof body === "string" ? body : JSON.stringify(body), init);
  }

  it("fetches and parses a valid manifest", async () => {
    const fake = validManifest();
    const fetchFn = async () => response(fake);
    const manifest = await fetchManifest("https://dl.example.com/app.inject.json", {
      fetch: fetchFn,
    });
    expect(manifest.board).toBe("pi-zero-2w");
  });

  it("throws GosdManifestFetchError on a non-ok response", async () => {
    const fetchFn = async () => response("nope", { status: 404 });
    await expect(
      fetchManifest("https://dl.example.com/app.inject.json", {
        fetch: fetchFn,
      }),
    ).rejects.toThrow(GosdManifestFetchError);
  });

  it("throws GosdManifestFetchError when fetch itself rejects", async () => {
    const fetchFn = async () => {
      throw new Error("network down");
    };
    await expect(
      fetchManifest("https://dl.example.com/app.inject.json", {
        fetch: fetchFn,
      }),
    ).rejects.toThrow(GosdManifestFetchError);
  });

  it("throws GosdManifestInvalidError on unparsable JSON", async () => {
    const fetchFn = async () => response("{not json");
    await expect(
      fetchManifest("https://dl.example.com/app.inject.json", {
        fetch: fetchFn,
      }),
    ).rejects.toThrow(GosdManifestInvalidError);
  });

  it("verifies manifestSha256 before parsing, and rejects on mismatch", async () => {
    const bytes = new TextEncoder().encode(JSON.stringify(validManifest()));
    const goodHash = new Sha256().update(bytes).digestHex();
    const fetchFn = async () => new Response(bytes);

    await expect(
      fetchManifest("https://dl.example.com/app.inject.json", {
        fetch: fetchFn,
        manifestSha256: goodHash,
      }),
    ).resolves.toBeTruthy();

    await expect(
      fetchManifest("https://dl.example.com/app.inject.json", {
        fetch: fetchFn,
        manifestSha256: "f".repeat(64),
      }),
    ).rejects.toThrow(GosdManifestHashMismatchError);
  });
});
