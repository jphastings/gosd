import { describe, expect, it } from "vitest";
import { padConfig, padContents } from "./content.js";
import { GosdContentTooLargeError, GosdUnknownPlaceholderError } from "./errors.js";
import type { Manifest } from "./manifest.js";

function manifestWith(placeholders: { path: string; size: number }[]): Manifest {
  return {
    gosd_inject: 1,
    board: "pi-zero-2w",
    image: { filename: "app.img", size: 1000, sha256: "a".repeat(64) },
    placeholders: placeholders.map((p) => ({
      path: p.path,
      size: p.size,
      sha256: "b".repeat(64),
      ranges: [{ offset: 0, length: p.size }],
    })),
  };
}

describe("padContents", () => {
  it("pads content shorter than the placeholder with trailing newlines, never NULs", () => {
    const manifest = manifestWith([{ path: "config.yaml", size: 10 }]);
    const padded = padContents({ "config.yaml": "hi" }, manifest);
    const out = padded.get("config.yaml")!;
    expect(out).toHaveLength(10);
    const nl = "\n".charCodeAt(0);
    expect(Array.from(out)).toEqual([
      "h".charCodeAt(0),
      "i".charCodeAt(0),
      nl,
      nl,
      nl,
      nl,
      nl,
      nl,
      nl,
      nl,
    ]);
  });

  it("uses content byte for byte on an exact-size fit", () => {
    const manifest = manifestWith([{ path: "config.yaml", size: 3 }]);
    const padded = padContents({ "config.yaml": "xyz" }, manifest);
    expect(Array.from(padded.get("config.yaml")!)).toEqual([120, 121, 122]);
  });

  it("handles an empty placeholder size with empty content", () => {
    const manifest = manifestWith([{ path: "empty", size: 0 }]);
    const padded = padContents({ empty: "" }, manifest);
    expect(padded.get("empty")).toHaveLength(0);
  });

  it("UTF-8 encodes multibyte string content before measuring size", () => {
    const manifest = manifestWith([{ path: "config.yaml", size: 10 }]);
    const padded = padContents({ "config.yaml": "héllo" }, manifest); // é is 2 bytes in UTF-8
    const out = padded.get("config.yaml")!;
    expect(out).toHaveLength(10);
    expect(new TextDecoder().decode(out.subarray(0, 6))).toBe("héllo");
  });

  it("accepts raw Uint8Array content unchanged (aside from padding)", () => {
    const manifest = manifestWith([{ path: "config.yaml", size: 4 }]);
    const padded = padContents({ "config.yaml": new Uint8Array([1, 2]) }, manifest);
    expect(Array.from(padded.get("config.yaml")!)).toEqual([1, 2, 0x0a, 0x0a]);
  });

  it("throws GosdContentTooLargeError naming both sizes when content overflows by one byte", () => {
    const manifest = manifestWith([{ path: "config.yaml", size: 4 }]);
    expect(() => padContents({ "config.yaml": "toolong" }, manifest)).toThrow(
      GosdContentTooLargeError,
    );
    expect(() => padContents({ "config.yaml": "toolong" }, manifest)).toThrow(/7 bytes.*4-byte/s);
  });

  it("throws GosdUnknownPlaceholderError listing available placeholders", () => {
    const manifest = manifestWith([
      { path: "config.yaml", size: 4 },
      { path: "net.cfg", size: 8 },
    ]);
    expect(() => padContents({ typo: "x" }, manifest)).toThrow(GosdUnknownPlaceholderError);
    expect(() => padContents({ typo: "x" }, manifest)).toThrow(/config\.yaml, net\.cfg/);
  });

  it("leaves placeholders not named in files untouched (not present in the result map)", () => {
    const manifest = manifestWith([
      { path: "config.yaml", size: 4 },
      { path: "net.cfg", size: 8 },
    ]);
    const padded = padContents({ "config.yaml": "ok" }, manifest);
    expect(padded.has("net.cfg")).toBe(false);
    expect(padded.size).toBe(1);
  });

  it("reports the unknown-placeholder error even with no placeholders at all", () => {
    const manifest = manifestWith([]);
    expect(() => padContents({ x: "y" }, manifest)).toThrow(/this image has no placeholders/);
  });
});

describe("padConfig", () => {
  const pristine = 'hostname = "device"\n';
  const withConfigRegion = (size: number): Manifest => ({
    gosd_inject: 1,
    board: "test",
    image: { filename: "f.img", size: 4096, sha256: "0".repeat(64) },
    placeholders: [],
    config: {
      path: "gosd.toml",
      size,
      sha256: "0".repeat(64),
      ranges: [{ offset: 0, length: size }],
      pristine: pristine.padEnd(size, "#"),
    },
  });

  it("pads a replacement to the reserved size with newlines", () => {
    const padded = padConfig('hostname = "other"\n', withConfigRegion(64));
    expect(padded.length).toBe(64);
    expect(new TextDecoder().decode(padded)).toBe('hostname = "other"\n'.padEnd(64, "\n"));
  });

  it("hands an edit function the pristine file it will replace", () => {
    let seen = "";
    padConfig((current) => {
      seen = current;
      return current;
    }, withConfigRegion(64));
    expect(seen).toBe(pristine.padEnd(64, "#"));
  });

  it("refuses an image that reserved no region, naming the flag that would", () => {
    const manifest = withConfigRegion(64);
    delete manifest.config;
    expect(() => padConfig('hostname = "x"\n', manifest)).toThrow(/--config-placeholder/);
  });

  it("refuses a config too large for the region, naming both sizes", () => {
    expect(() => padConfig('hostname = "a-very-long-name-indeed"\n', withConfigRegion(8))).toThrow(
      GosdContentTooLargeError,
    );
  });
});

describe("padContents with the config file's path", () => {
  it("points at the config option rather than treating it as a missing placeholder", () => {
    const manifest: Manifest = {
      gosd_inject: 1,
      board: "test",
      image: { filename: "f.img", size: 4096, sha256: "0".repeat(64) },
      placeholders: [],
      config: {
        path: "gosd.toml",
        size: 16,
        sha256: "0".repeat(64),
        ranges: [{ offset: 0, length: 16 }],
        pristine: "#".repeat(16),
      },
    };
    expect(() => padContents({ "gosd.toml": 'hostname = "x"' }, manifest)).toThrow(
      /config` option/,
    );
  });
});
