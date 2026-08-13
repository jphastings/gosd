// Download resuming on the fs-access tier (see the package README's
// "Resuming" section). Two halves:
//
//   - `createFreshDownloadCheckpoint` wires a fresh `withPlaceholders` fs-
//     access download (index.ts, opted in via `options.resumable`) up to a
//     ResumeStore, persisting just enough — the FileSystemFileHandle, the
//     image's identity and size, its ETag/Last-Modified, and each patched
//     placeholder's pristine bytes as they're verified — to make a later
//     `resumeDownload` call possible, without ever touching the base
//     SaveSink contract (see sinks/types.ts's SeekableSaveSink).
//   - `resumeDownload` (plus `listResumableDownloads` /
//     `discardResumableDownload`) picks a persisted record back up: it
//     reopens the same file handle, re-verifies the partial file already on
//     disk by reconstructing what its pristine bytes must have been (
//     `reconstructPristinePrefix`, swapping each captured placeholder's
//     stashed pristine bytes back in for whatever replacement content is
//     sitting there instead) and re-hashing that reconstruction
//     (`primeSubstitutionState`, in substitute.ts) rather than serializing
//     and restoring the vendored Sha256's internal state — then issues an
//     HTTP `Range` request (`If-Range` pinned to the stored ETag or
//     Last-Modified) and continues the substitution pass from the verified
//     offset. A server that ignores the `Range` (a plain `200`) restarts
//     the download from scratch, reusing the same already-picked file.

import { GosdImageFetchError, GosdImagePreconditionError, GosdSaveFailedError } from "./errors.js";
import { deriveManifestURL, fetchManifest, injectableRegions, type Manifest } from "./manifest.js";
import { padAll } from "./content.js";
import { primeSubstitutionState, type SubstitutionProgress } from "./substitute.js";
import { runDownload, type DownloadCheckpoint } from "./run.js";
import type { SeekableSaveSink } from "./sinks/types.js";
import { openPersistedFsAccessHandle } from "./sinks/fs-access.js";
import {
  createIndexedDbResumeStore,
  resumeStoreAvailable,
  type ResumeRecord,
  type ResumeStore,
} from "./resume-store.js";

export type { ResumeRecord, ResumeStore } from "./resume-store.js";
export { resumeStoreAvailable, createIndexedDbResumeStore } from "./resume-store.js";

/** `withPlaceholders`'s result shape, repeated here (rather than imported
 * from index.ts) to keep this module's dependency direction one-way — a
 * resumed download's `savedVia` is always `"fs-access"`, so this is
 * intentionally narrower than `WithPlaceholdersResult`. */
export interface ResumeDownloadResult {
  savedVia: "fs-access";
  manifest: Manifest;
  sha256: string;
  filename: string;
}

/** Rolls `offset` back to the start of any patched placeholder's range it
 * falls strictly inside, when that placeholder's pristine bytes weren't
 * fully captured as of this checkpoint — reconstructing the original bytes
 * for a straddled, incompletely-captured placeholder isn't possible, so
 * resuming from partway through it isn't safe. An untouched placeholder's
 * on-disk bytes are already pristine, so straddling one is fine regardless.
 * Placeholders' ranges are disjoint (the manifest guarantees it), so a
 * single pass suffices. */
export function clampToSafeResumeOffset(
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  capturedPristine: Record<string, Uint8Array>,
  offset: number,
): number {
  let clamped = offset;
  for (const region of injectableRegions(manifest)) {
    if (!padded.has(region.key) || region.key in capturedPristine) continue;
    for (const range of region.ranges) {
      const start = range.offset;
      const end = range.offset + range.length;
      if (offset > start && offset < end) {
        clamped = Math.min(clamped, start);
      }
    }
  }
  return clamped;
}

/** Reconstructs what the server's original (pristine) bytes must have been
 * for `prefixOnDisk` (the first `prefixOnDisk.length` bytes of the file as
 * currently written to disk) by swapping each fully-captured patched
 * placeholder's on-disk (replacement) bytes back out for the pristine
 * bytes stashed during the interrupted attempt. Everything outside a
 * patched placeholder's ranges is already pristine — only placeholder
 * ranges are ever rewritten — so it's copied through unchanged. A
 * placeholder whose captured range extends beyond `prefixOnDisk` (this
 * shouldn't happen for a correctly-checkpointed record, but a checkpoint
 * racing the underlying write is possible in principle — see fs-access.ts)
 * is left un-reconstructed; the resulting mismatch then fails verification
 * loudly in `primeSubstitutionState` rather than silently reconstructing
 * the wrong bytes. */
export function reconstructPristinePrefix(
  prefixOnDisk: Uint8Array,
  manifest: Manifest,
  capturedPristine: Record<string, Uint8Array>,
): Uint8Array {
  const out = Uint8Array.from(prefixOnDisk);
  for (const region of injectableRegions(manifest)) {
    const pristine = capturedPristine[region.key];
    if (!pristine) continue;
    let consumed = 0;
    for (const range of region.ranges) {
      const start = range.offset;
      const end = start + range.length;
      if (end > out.length) break;
      out.set(pristine.subarray(consumed, consumed + range.length), start);
      consumed += range.length;
    }
  }
  return out;
}

function resolveStore(store: ResumeStore | undefined): ResumeStore | undefined {
  if (store) return store;
  return resumeStoreAvailable() ? createIndexedDbResumeStore() : undefined;
}

/** Builds the `DownloadCheckpoint` both a fresh and a resumed download use
 * to keep `record` (already persisted by the caller) in sync with a
 * `runDownload` attempt in progress: updates it as the response headers
 * and each patched placeholder verify, and on finalization either deletes
 * it (full success) or checkpoints its true `bytesWritten` — read back
 * from the sink itself, not tracked separately, since that's the only
 * number that's actually durable (see run.ts's module doc). */
function buildCheckpoint(
  sink: SeekableSaveSink,
  record: ResumeRecord,
  store: ResumeStore,
): DownloadCheckpoint {
  return {
    onResponseHeaders(headers) {
      record.etag = headers.etag;
      record.lastModified = headers.lastModified;
      void store.put(record).catch(() => {});
    },
    onPlaceholderVerified(path, pristine) {
      record.pristinePlaceholders = { ...record.pristinePlaceholders, [path]: pristine };
      void store.put(record).catch(() => {});
    },
    async onFinalized(completed) {
      if (completed) {
        await store.delete(record.key).catch(() => {});
        return;
      }
      record.bytesWritten = (await sink.readExisting()).length;
      await store.put(record).catch(() => {});
    },
  };
}

export interface CreateFreshDownloadCheckpointOptions {
  sink: SeekableSaveSink;
  manifest: Manifest;
  imageURL: string;
  filename: string;
  /** Overrides the default IndexedDB-backed store — mainly for tests. */
  store?: ResumeStore;
}

/** Builds the `DownloadCheckpoint` a fresh `withPlaceholders` fs-access
 * download passes to `runDownload` when `options.resumable` is set:
 * persists an initial record immediately, updates it as placeholders
 * verify and the response headers are known, and either deletes it (full
 * success) or checkpoints its final `bytesWritten` (a preserved, recoverable
 * failure) when the attempt is finalized. Returns undefined when no
 * ResumeStore is available (no `indexedDB`) — resuming then just isn't
 * offered, same as before `options.resumable` existed. */
export function createFreshDownloadCheckpoint(
  options: CreateFreshDownloadCheckpointOptions,
): DownloadCheckpoint | undefined {
  const store = resolveStore(options.store);
  if (!store) return undefined;

  const record: ResumeRecord = {
    key: options.manifest.image.sha256,
    imageURL: options.imageURL,
    filename: options.filename,
    imageSize: options.manifest.image.size,
    etag: null,
    lastModified: null,
    bytesWritten: 0,
    pristinePlaceholders: {},
    handle: options.sink.resumeHandle,
  };
  void store.put(record).catch(() => {});

  return buildCheckpoint(options.sink, record, store);
}

export interface ResumableDownloadInfo {
  key: string;
  imageURL: string;
  filename: string;
  imageSize: number;
  bytesWritten: number;
}

export interface ListResumableDownloadsOptions {
  store?: ResumeStore;
}

/** Lists every download this browser has a resumable, persisted checkpoint
 * for — e.g. to offer the user a "resume previous download?" prompt.
 * Resolves to `[]` when no ResumeStore is available. */
export async function listResumableDownloads(
  options: ListResumableDownloadsOptions = {},
): Promise<ResumableDownloadInfo[]> {
  const store = resolveStore(options.store);
  if (!store) return [];
  const records = await store.list();
  return records.map((r) => ({
    key: r.key,
    imageURL: r.imageURL,
    filename: r.filename,
    imageSize: r.imageSize,
    bytesWritten: r.bytesWritten,
  }));
}

export interface DiscardResumableDownloadOptions {
  store?: ResumeStore;
}

/** Forgets a persisted resumable download without touching the partial
 * file itself — use when the corresponding `resumeDownload` failed
 * unrecoverably, or the user declined to resume. */
export async function discardResumableDownload(
  key: string,
  options: DiscardResumableDownloadOptions = {},
): Promise<void> {
  const store = resolveStore(options.store);
  if (!store) return;
  await store.delete(key);
}

export interface ResumeDownloadOptions {
  /** The `ResumeRecord.key` to resume — from `listResumableDownloads`. */
  key: string;
  files: Record<string, string | Uint8Array>;
  /** The config tree settings — pass the same ones the interrupted download
   * used, exactly as with `files`: a resume continues that attempt's bytes,
   * it doesn't re-decide them. */
  config?: Record<string, string>;
  manifestURL?: string | URL;
  manifestSha256?: string;
  manifest?: Manifest;
  fetch?: typeof fetch;
  signal?: AbortSignal;
  onProgress?: (progress: SubstitutionProgress) => void;
  /** Overrides the default IndexedDB-backed store — mainly for tests. */
  store?: ResumeStore;
}

/** Resumes a previously interrupted fs-access download identified by
 * `options.key`. Re-verifies whatever's already on disk before trusting
 * it, requests the rest of the image with `Range`/`If-Range`, and
 * continues the same substitution/verification pass a fresh download
 * would have run. Falls back to restarting from scratch — reusing the
 * same already-picked file, no new save picker — when the server ignores
 * the `Range` request (a plain `200`). Throws if no record is stored for
 * `key`, if the persisted file handle's permission is refused, or if the
 * partial file fails re-verification (in which case the caller should
 * `discardResumableDownload` and start over with `withPlaceholders`). */
export async function resumeDownload(
  options: ResumeDownloadOptions,
): Promise<ResumeDownloadResult> {
  const store = resolveStore(options.store);
  if (!store) {
    throw new GosdSaveFailedError(
      "resumeDownload needs a ResumeStore: this environment has no indexedDB and no options.store was given",
    );
  }

  const record = await store.get(options.key);
  if (!record) {
    throw new GosdSaveFailedError(
      `no resumable download is stored for key "${options.key}"; it may already have completed, been discarded, or never started`,
    );
  }

  const manifest =
    options.manifest ??
    (await fetchManifest(options.manifestURL ?? deriveManifestURL(record.imageURL), {
      fetch: options.fetch,
      manifestSha256: options.manifestSha256,
      signal: options.signal,
    }));
  if (manifest.image.sha256 !== record.key) {
    throw new GosdImagePreconditionError(
      `the manifest at the resumed image's URL now describes a different image (sha256 ${manifest.image.sha256}) than the interrupted download (${record.key}); discard this resumable download (discardResumableDownload) and start fresh`,
    );
  }
  const padded = padAll(options.files, options.config, manifest);

  const persisted = await openPersistedFsAccessHandle(record.handle);
  const onDiskOffset = Math.min(record.bytesWritten, persisted.existingBytes.length);
  const offset = clampToSafeResumeOffset(
    manifest,
    padded,
    record.pristinePlaceholders,
    onDiskOffset,
  );

  const doFetch = options.fetch ?? fetch;
  const ifRange = record.etag ?? record.lastModified ?? undefined;
  const { response, resumed, effectiveOffset } = await fetchRangeOrFull(
    doFetch,
    record.imageURL,
    offset,
    ifRange,
    options.signal,
  );

  const resumeFrom = resumed
    ? primeSubstitutionState(
        manifest,
        padded,
        reconstructPristinePrefix(
          persisted.existingBytes.subarray(0, effectiveOffset),
          manifest,
          record.pristinePlaceholders,
        ),
      )
    : undefined;
  const sink = resumed
    ? await persisted.resumeWritingAt(effectiveOffset)
    : await persisted.restartWriting();

  const result = await runDownload({
    manifest,
    padded,
    fetchImage: () => Promise.resolve(response),
    sink,
    signal: options.signal,
    onProgress: options.onProgress,
    resumeFrom,
    checkpoint: buildCheckpoint(sink, record, store),
  });

  return { savedVia: "fs-access", manifest, sha256: result.sha256, filename: record.filename };
}

interface FetchRangeOrFullResult {
  response: Response;
  resumed: boolean;
  effectiveOffset: number;
}

async function fetchRangeOrFull(
  doFetch: typeof fetch,
  url: string,
  offset: number,
  ifRange: string | undefined,
  signal: AbortSignal | undefined,
): Promise<FetchRangeOrFullResult> {
  if (offset === 0) {
    return { response: await doFetch(url, { signal }), resumed: false, effectiveOffset: 0 };
  }

  const headers = new Headers({ Range: `bytes=${offset}-` });
  if (ifRange) headers.set("If-Range", ifRange);

  const response = await doFetch(url, { headers, signal });
  if (response.status === 206) {
    return { response, resumed: true, effectiveOffset: offset };
  }
  if (response.status === 200) {
    // The server ignored the Range request — no support, or the If-Range
    // precondition failed because the resource changed — so it sent the
    // whole image; restart from scratch on this same response rather than
    // trusting a prefix that no longer matches what's being streamed.
    return { response, resumed: false, effectiveOffset: 0 };
  }
  throw new GosdImageFetchError(
    `resuming the image download failed: the server responded with HTTP ${response.status} to a Range request`,
  );
}
