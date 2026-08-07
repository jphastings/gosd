// Tier 1: the File System Access API. `showSaveFilePicker` must be called
// synchronously within a user gesture (no `await` ahead of it) — Chromium
// requires "transient user activation" for it, and JS async functions run
// synchronously up to their first `await`, so calling this as the very
// first thing withPlaceholders does (before fetchManifest or anything else
// async) keeps the call inside the click handler's activation window. See
// run.test.ts / index.test.ts for the call-order test pinning this.
//
// Chromium stages writes to a swap file that's only published to the real
// path on `close()`, so a failure here never leaves partial image bytes at
// the user-visible path — cleanup is `writable.abort()` plus a best-effort
// call to the non-standard `handle.remove()` (worst case: an empty file,
// which the thrown error names).
//
// That swap-file behavior also bounds what "resuming" can mean here: bytes
// streamed through a writable that's never been closed aren't visible via
// `handle.getFile()` at all (they're sitting in the swap file, not the
// real path) — so a resume is only ever as good as the last point this
// tier's own code explicitly closed the writable (see run.ts's
// `checkpoint`/`DownloadCheckpoint.onFinalized`, which does exactly that on
// a recoverable failure), never an uncontrolled crash mid-stream.

import { GosdCancelledError, GosdSaveFailedError } from "../errors.js";
import type { SeekableSaveSink } from "./types.js";

// The File System Access API isn't part of TypeScript's built-in DOM lib
// yet, so the handful of types used here are hand-declared rather than
// pulled from a devDependency — this package ships zero runtime
// dependencies, and keeps its type-only ones to exactly what's listed in
// js/package.json.
interface FileSystemWritableFileStream extends WritableStream<Uint8Array> {
  write(data: Uint8Array): Promise<void>;
  // Standard on FileSystemWritableFileStream, but only meaningful (and
  // only declared here as optional) when the stream was opened with
  // `keepExistingData: true` — moves the write cursor without touching
  // what's already there, which is how a resume continues past the bytes
  // it already trusts.
  seek?(position: number): Promise<void>;
}

type PermissionState = "granted" | "denied" | "prompt";

interface FileSystemFileHandle {
  createWritable(options?: { keepExistingData?: boolean }): Promise<FileSystemWritableFileStream>;
  getFile(): Promise<{ size: number; arrayBuffer(): Promise<ArrayBuffer> }>;
  // Non-standard Chromium extension used for best-effort cleanup; absent
  // elsewhere (where this tier isn't auto-selected anyway).
  remove?(): Promise<void>;
  // Standardized permissions extension, needed to keep writing to a handle
  // persisted (via resume-store.ts) across a page reload — a fresh
  // showSaveFilePicker() call implicitly grants "readwrite", but that
  // grant doesn't survive the browsing session, so resuming re-requests
  // it. Absent handles (this optional chaining) are treated as already
  // permitted, matching a browser that doesn't implement this extension.
  queryPermission?(descriptor?: { mode?: "read" | "readwrite" }): Promise<PermissionState>;
  requestPermission?(descriptor?: { mode?: "read" | "readwrite" }): Promise<PermissionState>;
}

interface ShowSaveFilePickerOptions {
  suggestedName?: string;
}

type ShowSaveFilePicker = (options?: ShowSaveFilePickerOptions) => Promise<FileSystemFileHandle>;

function getShowSaveFilePicker(target: unknown): ShowSaveFilePicker | undefined {
  const picker = (target as { showSaveFilePicker?: unknown }).showSaveFilePicker;
  return typeof picker === "function" ? (picker as ShowSaveFilePicker) : undefined;
}

/** True when `target` (defaults to `globalThis`) exposes
 * `showSaveFilePicker` — the auto tier-selection's tier-1 gate. */
export function fsAccessAvailable(target: unknown = globalThis): boolean {
  return getShowSaveFilePicker(target) !== undefined;
}

function wrapHandle(
  handle: FileSystemFileHandle,
  writable: FileSystemWritableFileStream,
): SeekableSaveSink {
  return {
    kind: "fs-access",
    writable,
    resumeHandle: handle,
    async readExisting() {
      const file = await handle.getFile();
      return new Uint8Array(await file.arrayBuffer());
    },
    async commit() {
      // runDownload's pipeTo already closes `writable` on success (the
      // Streams spec default, which is also what publishes Chromium's
      // staged swap file to the real path) — this is a defensive fallback
      // for anyone driving the sink without pipeTo, tolerant of the
      // already-closed rejection the normal path leaves behind.
      try {
        await writable.close();
      } catch {
        // Already closed by pipeTo; nothing more to do.
      }
    },
    async abort(reason) {
      try {
        await writable.abort(reason);
      } catch {
        // Already closed or aborted; nothing more to do.
      }
      try {
        await handle.remove?.();
      } catch {
        // Best-effort: worst case is a 0-byte file, which the caller's
        // thrown error already names.
      }
    },
  };
}

async function ensureReadWritePermission(handle: FileSystemFileHandle): Promise<void> {
  if (!handle.requestPermission) return; // no permissions extension: assume already permitted
  const state = await handle.requestPermission({ mode: "readwrite" });
  if (state !== "granted") {
    throw new GosdSaveFailedError(
      `permission to continue writing to the previously chosen file was not granted (${state}); discard this resumable download (discardResumableDownload) and start a fresh one`,
    );
  }
}

export interface CreateFsAccessSinkOptions {
  suggestedName: string;
}

/** Opens the browser's native save picker and wraps the resulting file
 * handle as a SaveSink. Rejects with `GosdCancelledError` if the user
 * dismisses the picker — nothing has been fetched yet at that point, so
 * there's nothing to clean up. */
export async function createFsAccessSink(
  options: CreateFsAccessSinkOptions,
  target: unknown = globalThis,
): Promise<SeekableSaveSink> {
  const showSaveFilePicker = getShowSaveFilePicker(target);
  if (!showSaveFilePicker) {
    throw new GosdSaveFailedError(
      "the fs-access save tier needs window.showSaveFilePicker, which this browser doesn't provide; pass options.saveVia to pick a different tier, or let withPlaceholders auto-select one",
    );
  }

  let handle: FileSystemFileHandle;
  try {
    handle = await showSaveFilePicker({ suggestedName: options.suggestedName });
  } catch (cause) {
    if ((cause as { name?: string } | undefined)?.name === "AbortError") {
      throw new GosdCancelledError("the save dialog was dismissed before anything was downloaded", {
        cause,
      });
    }
    throw new GosdSaveFailedError(`opening the save dialog for "${options.suggestedName}" failed`, {
      cause,
    });
  }

  const writable = await handle.createWritable();
  return wrapHandle(handle, writable);
}

/** What a persisted (`SeekableSaveSink.resumeHandle`) fs-access destination
 * offers a resumed download: the bytes already on disk, to re-verify (see
 * resume.ts), and two ways to reopen it — continuing past a trusted offset,
 * or starting over when the server won't honor a `Range` request. Both
 * re-request the "readwrite" permission once, up front, since it doesn't
 * survive a page reload. */
export interface PersistedFsAccessHandle {
  existingBytes: Uint8Array;
  resumeWritingAt(offset: number): Promise<SeekableSaveSink>;
  restartWriting(): Promise<SeekableSaveSink>;
}

/** Reopens a persisted `resumeHandle` (as stored in a `ResumeRecord`) for a
 * resumed download. Throws `GosdSaveFailedError` if the handle isn't
 * usable (not a real file handle, or permission is denied) — the caller
 * should treat that as "this resumable download can't continue; discard it
 * and start fresh". */
export async function openPersistedFsAccessHandle(
  resumeHandle: unknown,
): Promise<PersistedFsAccessHandle> {
  const handle = resumeHandle as Partial<FileSystemFileHandle> | null | undefined;
  if (
    !handle ||
    typeof handle.createWritable !== "function" ||
    typeof handle.getFile !== "function"
  ) {
    throw new GosdSaveFailedError(
      "the persisted resume handle is not a usable file handle; discard this resumable download (discardResumableDownload) and start a fresh one",
    );
  }
  const fullHandle = handle as FileSystemFileHandle;
  await ensureReadWritePermission(fullHandle);

  const file = await fullHandle.getFile();
  const existingBytes = new Uint8Array(await file.arrayBuffer());

  return {
    existingBytes,
    async resumeWritingAt(offset: number) {
      const writable = await fullHandle.createWritable({ keepExistingData: true });
      if (writable.seek) {
        await writable.seek(offset);
      } else if (offset !== 0) {
        throw new GosdSaveFailedError(
          "this browser's File System Access implementation can't seek a writable stream, which resuming a non-empty download needs; discard this resumable download and start a fresh one",
        );
      }
      return wrapHandle(fullHandle, writable);
    },
    async restartWriting() {
      const writable = await fullHandle.createWritable();
      return wrapHandle(fullHandle, writable);
    },
  };
}
