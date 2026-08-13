import { describe, expect, it } from "vitest";
import { padConfig, padContents } from "./content.js";
import {
  GosdContentTooLargeError,
  GosdInvalidEnvError,
  GosdUnknownConfigError,
  GosdUnknownPlaceholderError,
} from "./errors.js";
import { configRegionKey, type Manifest } from "./manifest.js";

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
    config: [],
  };
}

function manifestWithSettings(settings: { path: string; size: number }[]): Manifest {
  return {
    gosd_inject: 1,
    board: "pi-zero-2w",
    image: { filename: "app.img", size: 1000, sha256: "a".repeat(64) },
    placeholders: [],
    config: settings.map((c, i) => ({
      path: c.path,
      size: c.size,
      sha256: "b".repeat(64),
      ranges: [{ offset: i * 100, length: c.size }],
      value: "",
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
  it("pads a setting's value to its reservation with trailing newlines", () => {
    const manifest = manifestWithSettings([{ path: "wifi/ssid", size: 8 }]);
    const padded = padConfig({ "wifi/ssid": "home" }, manifest);
    const out = padded.get(configRegionKey("wifi/ssid"))!;
    expect(new TextDecoder().decode(out)).toBe("home\n\n\n\n");
  });

  it("lists the image's settings when the path isn't one of them", () => {
    const manifest = manifestWithSettings([
      { path: "wifi/ssid", size: 8 },
      { path: "hostname", size: 8 },
    ]);
    expect(() => padConfig({ "wifi/sid": "home" }, manifest)).toThrow(GosdUnknownConfigError);
    expect(() => padConfig({ "wifi/sid": "home" }, manifest)).toThrow(/wifi\/ssid, hostname/);
  });

  it("refuses an image with no settings at all", () => {
    expect(() => padConfig({ "wifi/ssid": "home" }, manifestWithSettings([]))).toThrow(
      GosdUnknownConfigError,
    );
  });

  it("refuses a value longer than the setting's reservation", () => {
    const manifest = manifestWithSettings([{ path: "wifi/ssid", size: 4 }]);
    expect(() => padConfig({ "wifi/ssid": "a-long-network-name" }, manifest)).toThrow(
      GosdContentTooLargeError,
    );
  });

  it("refuses a GOSD_* environment variable the device would ignore", () => {
    const manifest = manifestWithSettings([{ path: "env/GOSD_BOARD", size: 32 }]);
    expect(() => padConfig({ "env/GOSD_BOARD": "pi-zero-2w" }, manifest)).toThrow(
      GosdInvalidEnvError,
    );
  });

  it("refuses an environment variable name no environment could carry", () => {
    const manifest = manifestWithSettings([{ path: "env/2FAST", size: 32 }]);
    expect(() => padConfig({ "env/2FAST": "x" }, manifest)).toThrow(GosdInvalidEnvError);
  });
});

describe("padContents given a setting's path", () => {
  it("points at the config option rather than just calling it unknown", () => {
    const manifest = manifestWithSettings([{ path: "wifi/ssid", size: 8 }]);
    expect(() => padContents({ "wifi/ssid": "home" }, manifest)).toThrow(/`config` option/);
  });
});
