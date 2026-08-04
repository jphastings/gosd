import { afterEach, describe, expect, it, vi } from "vitest";
import { GosdSaveFailedError } from "../errors.js";
import { createMemorySink } from "./memory.js";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

interface FakeAnchor {
  style: Record<string, string>;
  href: string;
  download: string;
  click(): void;
}

// Only `document` is stubbed — Node has no DOM at all, but its global `URL`
// already implements createObjectURL/revokeObjectURL for real Blobs, so
// those are exercised for real (and spied on) rather than faked too.
function stubDocument(): { clicked: FakeAnchor[] } {
  const clicked: FakeAnchor[] = [];
  const fakeAnchor: FakeAnchor = {
    style: {},
    href: "",
    download: "",
    click() {
      clicked.push({ ...fakeAnchor });
    },
  };
  vi.stubGlobal("document", {
    createElement: () => fakeAnchor,
    body: { appendChild: () => {}, removeChild: () => {} },
  });
  return { clicked };
}

describe("createMemorySink", () => {
  it("accumulates written chunks and triggers a Blob download on commit", async () => {
    const { clicked } = stubDocument();
    const revokeSpy = vi.spyOn(URL, "revokeObjectURL");
    vi.useFakeTimers();

    const sink = createMemorySink({ filename: "app.img" });
    const writer = sink.writable.getWriter();
    await writer.write(new Uint8Array([1, 2, 3]));
    await writer.write(new Uint8Array([4, 5]));
    await writer.close();
    await sink.commit();

    expect(clicked).toHaveLength(1);
    expect(clicked[0]?.href).toMatch(/^blob:/);
    expect(clicked[0]?.download).toBe("app.img");

    vi.runAllTimers();
    expect(revokeSpy).toHaveBeenCalledWith(clicked[0]?.href);
    vi.useRealTimers();
  });

  it("abort() clears buffered chunks; a later commit() is then a no-op", async () => {
    const sink = createMemorySink({ filename: "app.img" });
    const writer = sink.writable.getWriter();
    await writer.write(new Uint8Array([1]));
    await sink.abort(new Error("boom"));

    const { clicked } = stubDocument();
    await sink.commit();
    expect(clicked).toHaveLength(0);
  });

  it("wraps a DOM-less environment's failure as GosdSaveFailedError", async () => {
    vi.stubGlobal("document", undefined);
    const sink = createMemorySink({ filename: "app.img" });
    const writer = sink.writable.getWriter();
    await writer.write(new Uint8Array([1]));
    await writer.close();

    await expect(sink.commit()).rejects.toThrow(GosdSaveFailedError);
  });
});
