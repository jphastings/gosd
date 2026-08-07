// Cross-session persistence for resumable fs-access downloads. A
// FileSystemFileHandle is structured-clonable — IndexedDB stores it
// natively, no serialization format needed — which is what makes
// persisting "the file the user already picked" across a page reload
// possible at all; `localStorage` can't hold one (string-only) and neither
// can anything that round-trips through JSON. This module only deals in
// opaque records: it doesn't know what a `handle` is or does, only
// sinks/fs-access.ts (the only tier resuming is scoped to, see the bean)
// interprets one.
//
// IndexedDB, not the File System Access API's own newer
// `getUserData`-style extensions: those aren't implemented broadly enough
// yet, whereas IndexedDB (including its support for storing
// FileSystemFileHandle values) already is in every browser this package's
// fs-access tier auto-selects on.

export interface ResumeRecord {
  /** The interrupted download's identity — `manifest.image.sha256`, so a
   * later resume automatically stops matching once the image at that URL
   * changes (a new manifest means a new key; the stale record is simply
   * never found again, and can be cleaned up with `discardResumableDownload`). */
  key: string;
  imageURL: string;
  filename: string;
  imageSize: number;
  etag: string | null;
  lastModified: string | null;
  /** How many bytes were durably on disk as of the last checkpoint — never
   * more than what's actually there; re-checked against the real file
   * before being trusted (see resume.ts). */
  bytesWritten: number;
  /** Pristine (pre-substitution) bytes for every patched placeholder whose
   * full range had already been durably written as of the last checkpoint,
   * keyed by placeholder path — small (placeholders are KiB-scale), and
   * exactly what reconstructing the equivalent original bytes for
   * re-verification needs (see resume.ts's `reconstructPristinePrefix`). */
  pristinePlaceholders: Record<string, Uint8Array>;
  /** Opaque, tier-specific persisted destination handle — see
   * `SeekableSaveSink.resumeHandle`. Only sinks/fs-access.ts interprets
   * this. */
  handle: unknown;
}

export interface ResumeStore {
  get(key: string): Promise<ResumeRecord | undefined>;
  put(record: ResumeRecord): Promise<void>;
  delete(key: string): Promise<void>;
  list(): Promise<ResumeRecord[]>;
}

const DB_NAME = "gosd-downloads-resume";
const STORE_NAME = "resumable";
const DB_VERSION = 1;

function getIndexedDB(target: unknown): IDBFactory | undefined {
  const candidate = (target as { indexedDB?: unknown } | undefined)?.indexedDB;
  return candidate as IDBFactory | undefined;
}

/** True when `target` (defaults to `globalThis`) exposes `indexedDB` — the
 * resumable-download feature's storage-availability gate, mirroring
 * `fsAccessAvailable`'s role for the save tier itself. */
export function resumeStoreAvailable(target: unknown = globalThis): boolean {
  return getIndexedDB(target) !== undefined;
}

function promisifyRequest<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("an IndexedDB request failed"));
  });
}

function openDatabase(indexedDB: IDBFactory): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      request.result.createObjectStore(STORE_NAME, { keyPath: "key" });
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () =>
      reject(request.error ?? new Error("failed to open the gosd resumable-downloads database"));
  });
}

/** An IndexedDB-backed `ResumeStore`. `target` defaults to `globalThis`;
 * pass a fake for tests, the same pattern `fsAccessAvailable`'s `target`
 * uses. Throws if this environment has no `indexedDB` — check
 * `resumeStoreAvailable()` first. */
export function createIndexedDbResumeStore(target: unknown = globalThis): ResumeStore {
  const factory = getIndexedDB(target);
  if (!factory) {
    throw new Error(
      "createIndexedDbResumeStore: this environment has no indexedDB; check resumeStoreAvailable() before calling this",
    );
  }
  // Reassigned to its own binding (rather than using `factory` directly
  // below) so its type is `IDBFactory`, not `IDBFactory | undefined` —
  // narrowing from the guard above doesn't reach into the closures below.
  const indexedDB: IDBFactory = factory;

  async function withStore<T>(
    mode: IDBTransactionMode,
    fn: (store: IDBObjectStore) => IDBRequest<T>,
  ): Promise<T> {
    const db = await openDatabase(indexedDB);
    try {
      const tx = db.transaction(STORE_NAME, mode);
      const store = tx.objectStore(STORE_NAME);
      return await promisifyRequest(fn(store));
    } finally {
      db.close();
    }
  }

  return {
    async get(key: string) {
      return (await withStore("readonly", (store) => store.get(key))) as ResumeRecord | undefined;
    },
    async put(record: ResumeRecord) {
      await withStore("readwrite", (store) => store.put(record));
    },
    async delete(key: string) {
      await withStore("readwrite", (store) => store.delete(key));
    },
    async list() {
      return (await withStore("readonly", (store) => store.getAll())) as ResumeRecord[];
    },
  };
}
