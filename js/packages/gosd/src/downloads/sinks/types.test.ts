import { describe, expect, it } from "vitest";
import { isSeekable } from "./types.js";
import type { SaveSink, SeekableSaveSink } from "./types.js";

function baseSink(): SaveSink {
  return {
    kind: "memory",
    writable: new WritableStream<Uint8Array>(),
    async commit() {},
    async abort() {},
  };
}

describe("isSeekable", () => {
  it("is false for a plain SaveSink", () => {
    expect(isSeekable(baseSink())).toBe(false);
  });

  it("is true for a sink that also implements readExisting/resumeHandle", () => {
    const sink: SeekableSaveSink = {
      ...baseSink(),
      resumeHandle: { some: "handle" },
      async readExisting() {
        return new Uint8Array();
      },
    };
    expect(isSeekable(sink)).toBe(true);
  });
});
