import { describe, expect, it } from "vitest";
import {
  createIndexedDbResumeStore,
  resumeStoreAvailable,
  type ResumeRecord,
} from "./resume-store.js";

// A minimal, in-memory fake of just the IndexedDB surface
// createIndexedDbResumeStore uses: open() with onupgradeneeded/onsuccess,
// one object store keyed by "key", and get/put/delete/getAll requests. Real
// IndexedDB's request callbacks fire asynchronously even for data that's
// already resident, which this mirrors with a microtask (`queueMicrotask`)
// rather than resolving synchronously.
function fakeIndexedDB(): { indexedDB: IDBFactory; data: Map<string, ResumeRecord> } {
  const data = new Map<string, ResumeRecord>();

  function makeRequest<T>(run: () => T): IDBRequest<T> {
    const request = {
      onsuccess: null,
      onerror: null,
      result: undefined,
      error: null,
    } as unknown as IDBRequest<T>;
    queueMicrotask(() => {
      try {
        (request as { result: T }).result = run();
        request.onsuccess?.(new Event("success"));
      } catch (err) {
        (request as { error: unknown }).error = err;
        request.onerror?.(new Event("error"));
      }
    });
    return request;
  }

  const store = {
    get: (key: string) => makeRequest(() => data.get(key)),
    put: (record: ResumeRecord) => makeRequest(() => void data.set(record.key, record)),
    delete: (key: string) => makeRequest(() => void data.delete(key)),
    getAll: () => makeRequest(() => Array.from(data.values())),
  };

  const db = {
    transaction: () => ({ objectStore: () => store }),
    close: () => {},
    createObjectStore: () => store,
  } as unknown as IDBDatabase;

  const indexedDB = {
    open: () => {
      const request = {
        onsuccess: null,
        onerror: null,
        onupgradeneeded: null,
        result: db,
      } as unknown as IDBOpenDBRequest;
      queueMicrotask(() => {
        request.onupgradeneeded?.(new Event("upgradeneeded") as never);
        request.onsuccess?.(new Event("success"));
      });
      return request;
    },
  } as unknown as IDBFactory;

  return { indexedDB, data };
}

function record(overrides: Partial<ResumeRecord> = {}): ResumeRecord {
  return {
    key: "abc123",
    imageURL: "https://dl.example.com/app.img",
    filename: "app.img",
    imageSize: 1000,
    etag: null,
    lastModified: null,
    bytesWritten: 0,
    pristinePlaceholders: {},
    handle: { some: "handle" },
    ...overrides,
  };
}

describe("resumeStoreAvailable", () => {
  it("is false when the target has no indexedDB", () => {
    expect(resumeStoreAvailable({})).toBe(false);
  });

  it("is true when the target exposes indexedDB", () => {
    const { indexedDB } = fakeIndexedDB();
    expect(resumeStoreAvailable({ indexedDB })).toBe(true);
  });
});

describe("createIndexedDbResumeStore", () => {
  it("throws when the environment has no indexedDB", () => {
    expect(() => createIndexedDbResumeStore({})).toThrow(/indexedDB/);
  });

  it("get() resolves to undefined for a key that was never stored", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });

    await expect(store.get("missing")).resolves.toBeUndefined();
  });

  it("put() then get() round-trips a record, including its Uint8Array fields", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });
    const r = record({ pristinePlaceholders: { "cloud-init.yaml": new Uint8Array([1, 2, 3]) } });

    await store.put(r);

    await expect(store.get(r.key)).resolves.toEqual(r);
  });

  it("put() again overwrites the previous record for the same key", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });
    await store.put(record({ bytesWritten: 10 }));
    await store.put(record({ bytesWritten: 20 }));

    await expect(store.get("abc123")).resolves.toMatchObject({ bytesWritten: 20 });
  });

  it("delete() removes a record", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });
    await store.put(record());

    await store.delete("abc123");

    await expect(store.get("abc123")).resolves.toBeUndefined();
  });

  it("delete() of a missing key is a no-op, not an error", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });

    await expect(store.delete("missing")).resolves.toBeUndefined();
  });

  it("list() returns every stored record", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });
    await store.put(record({ key: "one" }));
    await store.put(record({ key: "two" }));

    const all = await store.list();

    expect(all.map((r) => r.key).sort()).toEqual(["one", "two"]);
  });

  it("list() is empty for a fresh store", async () => {
    const { indexedDB } = fakeIndexedDB();
    const store = createIndexedDbResumeStore({ indexedDB });

    await expect(store.list()).resolves.toEqual([]);
  });
});
