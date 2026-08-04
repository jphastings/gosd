import { describe, expect, it, vi } from "vitest";
import { GosdCancelledError, GosdSaveFailedError } from "../errors.js";
import { createFsAccessSink, fsAccessAvailable } from "./fs-access.js";

function fakeWritable() {
  const close = vi.fn(async () => {});
  const abort = vi.fn(async () => {});
  return { close, abort, stream: { close, abort } };
}

describe("fsAccessAvailable", () => {
  it("is false when the target has no showSaveFilePicker", () => {
    expect(fsAccessAvailable({})).toBe(false);
  });

  it("is true when the target exposes a showSaveFilePicker function", () => {
    expect(fsAccessAvailable({ showSaveFilePicker: async () => ({}) })).toBe(true);
  });
});

describe("createFsAccessSink", () => {
  it("calls showSaveFilePicker with the suggested name", async () => {
    const writable = fakeWritable();
    const showSaveFilePicker = vi.fn(async () => ({
      createWritable: async () => writable.stream,
    }));

    await createFsAccessSink({ suggestedName: "app.img" }, { showSaveFilePicker });

    expect(showSaveFilePicker).toHaveBeenCalledWith({
      suggestedName: "app.img",
    });
  });

  it("throws GosdCancelledError when the picker is dismissed (AbortError)", async () => {
    const target = {
      showSaveFilePicker: async () => {
        const err = new Error("dismissed");
        err.name = "AbortError";
        throw err;
      },
    };

    await expect(createFsAccessSink({ suggestedName: "app.img" }, target)).rejects.toThrow(
      GosdCancelledError,
    );
  });

  it("throws GosdSaveFailedError when showSaveFilePicker isn't available", async () => {
    await expect(createFsAccessSink({ suggestedName: "app.img" }, {})).rejects.toThrow(
      GosdSaveFailedError,
    );
  });

  it("throws GosdSaveFailedError for a non-cancellation picker failure", async () => {
    const target = {
      showSaveFilePicker: async () => {
        throw new Error("disk full");
      },
    };
    await expect(createFsAccessSink({ suggestedName: "app.img" }, target)).rejects.toThrow(
      GosdSaveFailedError,
    );
  });

  it("commit() closes the underlying writable", async () => {
    const writable = fakeWritable();
    const target = {
      showSaveFilePicker: async () => ({
        createWritable: async () => writable.stream,
      }),
    };
    const sink = await createFsAccessSink({ suggestedName: "app.img" }, target);

    await sink.commit();
    expect(writable.close).toHaveBeenCalledTimes(1);
  });

  it("abort() aborts the writable and best-effort removes the handle", async () => {
    const writable = fakeWritable();
    const remove = vi.fn(async () => {});
    const target = {
      showSaveFilePicker: async () => ({
        createWritable: async () => writable.stream,
        remove,
      }),
    };
    const sink = await createFsAccessSink({ suggestedName: "app.img" }, target);

    await sink.abort(new Error("boom"));
    expect(writable.abort).toHaveBeenCalledWith(expect.any(Error));
    expect(remove).toHaveBeenCalledTimes(1);
  });

  it("abort() tolerates a missing non-standard remove()", async () => {
    const writable = fakeWritable();
    const target = {
      showSaveFilePicker: async () => ({
        createWritable: async () => writable.stream,
      }),
    };
    const sink = await createFsAccessSink({ suggestedName: "app.img" }, target);

    await expect(sink.abort(new Error("boom"))).resolves.toBeUndefined();
  });

  it("abort() tolerates the writable already being aborted/closed", async () => {
    const writable = fakeWritable();
    writable.abort.mockRejectedValueOnce(new Error("already aborted"));
    const target = {
      showSaveFilePicker: async () => ({
        createWritable: async () => writable.stream,
      }),
    };
    const sink = await createFsAccessSink({ suggestedName: "app.img" }, target);

    await expect(sink.abort(new Error("boom"))).resolves.toBeUndefined();
  });
});
