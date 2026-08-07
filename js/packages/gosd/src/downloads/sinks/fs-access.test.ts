import { describe, expect, it, vi } from "vitest";
import { GosdCancelledError, GosdSaveFailedError } from "../errors.js";
import { createFsAccessSink, fsAccessAvailable, openPersistedFsAccessHandle } from "./fs-access.js";

function fakeWritable() {
  const close = vi.fn(async () => {});
  const abort = vi.fn(async () => {});
  return { close, abort, stream: { close, abort } };
}

function fakeHandle(
  overrides: Partial<{
    bytes: Uint8Array;
    seek: ReturnType<typeof vi.fn>;
    requestPermission: ReturnType<typeof vi.fn>;
  }> = {},
) {
  const bytes = overrides.bytes ?? new Uint8Array([1, 2, 3]);
  const seek = "seek" in overrides ? overrides.seek : vi.fn(async () => {});
  const createWritable = vi.fn(async () => ({
    write: vi.fn(async () => {}),
    close: vi.fn(async () => {}),
    abort: vi.fn(async () => {}),
    ...(seek ? { seek } : {}),
  }));
  const getFile = vi.fn(async () => ({
    size: bytes.length,
    arrayBuffer: async () => bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.length),
  }));
  const requestPermission = overrides.requestPermission ?? vi.fn(async () => "granted");
  return { createWritable, getFile, requestPermission };
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

  it("exposes the picked handle as resumeHandle, and reads it back via readExisting()", async () => {
    const handle = fakeHandle({ bytes: new Uint8Array([9, 9, 9]) });
    const target = { showSaveFilePicker: async () => handle };

    const sink = await createFsAccessSink({ suggestedName: "app.img" }, target);

    expect(sink.resumeHandle).toBe(handle);
    await expect(sink.readExisting()).resolves.toEqual(new Uint8Array([9, 9, 9]));
  });
});

describe("openPersistedFsAccessHandle", () => {
  it("throws GosdSaveFailedError for a handle that isn't a usable file handle", async () => {
    await expect(openPersistedFsAccessHandle({})).rejects.toThrow(GosdSaveFailedError);
    await expect(openPersistedFsAccessHandle(null)).rejects.toThrow(GosdSaveFailedError);
    await expect(openPersistedFsAccessHandle(undefined)).rejects.toThrow(GosdSaveFailedError);
  });

  it("requests readwrite permission once, up front, and exposes existingBytes", async () => {
    const handle = fakeHandle({ bytes: new Uint8Array([1, 2, 3, 4]) });

    const persisted = await openPersistedFsAccessHandle(handle);

    expect(handle.requestPermission).toHaveBeenCalledExactlyOnceWith({ mode: "readwrite" });
    expect(persisted.existingBytes).toEqual(new Uint8Array([1, 2, 3, 4]));
  });

  it("throws GosdSaveFailedError when permission is refused", async () => {
    const handle = fakeHandle({ requestPermission: vi.fn(async () => "denied") });

    await expect(openPersistedFsAccessHandle(handle)).rejects.toThrow(GosdSaveFailedError);
  });

  it("tolerates a handle with no requestPermission extension (assumes already permitted)", async () => {
    const handle = fakeHandle();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- deleting a fake's own method for the test
    delete (handle as any).requestPermission;

    await expect(openPersistedFsAccessHandle(handle)).resolves.toBeDefined();
  });

  it("resumeWritingAt(offset) reopens with keepExistingData and seeks to offset", async () => {
    const handle = fakeHandle();
    const persisted = await openPersistedFsAccessHandle(handle);

    const sink = await persisted.resumeWritingAt(42);

    expect(handle.createWritable).toHaveBeenCalledWith({ keepExistingData: true });
    const writable = await handle.createWritable.mock.results[0]?.value;
    expect(writable.seek).toHaveBeenCalledExactlyOnceWith(42);
    expect(sink.kind).toBe("fs-access");
  });

  it("resumeWritingAt(0) works even without seek support", async () => {
    const handle = fakeHandle({ seek: undefined });
    const persisted = await openPersistedFsAccessHandle(handle);

    await expect(persisted.resumeWritingAt(0)).resolves.toBeDefined();
  });

  it("resumeWritingAt(offset > 0) throws when the writable can't seek", async () => {
    const handle = fakeHandle({ seek: undefined });
    const persisted = await openPersistedFsAccessHandle(handle);

    await expect(persisted.resumeWritingAt(10)).rejects.toThrow(GosdSaveFailedError);
  });

  it("restartWriting() reopens without keepExistingData, for a from-scratch restart", async () => {
    const handle = fakeHandle();
    const persisted = await openPersistedFsAccessHandle(handle);

    const sink = await persisted.restartWriting();

    expect(handle.createWritable).toHaveBeenCalledWith();
    expect(sink.kind).toBe("fs-access");
  });
});
