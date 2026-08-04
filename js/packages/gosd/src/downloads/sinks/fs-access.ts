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

import { GosdCancelledError, GosdSaveFailedError } from "../errors.js";
import type { SaveSink } from "./types.js";

// The File System Access API isn't part of TypeScript's built-in DOM lib
// yet, so the handful of types used here are hand-declared rather than
// pulled from a devDependency — this package ships zero runtime
// dependencies, and keeps its type-only ones to exactly what's listed in
// js/package.json.
interface FileSystemWritableFileStream extends WritableStream<Uint8Array> {
  write(data: Uint8Array): Promise<void>;
}

interface FileSystemFileHandle {
  createWritable(): Promise<FileSystemWritableFileStream>;
  // Non-standard Chromium extension used for best-effort cleanup; absent
  // elsewhere (where this tier isn't auto-selected anyway).
  remove?(): Promise<void>;
}

interface ShowSaveFilePickerOptions {
  suggestedName?: string;
}

type ShowSaveFilePicker = (
  options?: ShowSaveFilePickerOptions,
) => Promise<FileSystemFileHandle>;

function getShowSaveFilePicker(
  target: unknown,
): ShowSaveFilePicker | undefined {
  const picker = (target as { showSaveFilePicker?: unknown })
    .showSaveFilePicker;
  return typeof picker === "function"
    ? (picker as ShowSaveFilePicker)
    : undefined;
}

/** True when `target` (defaults to `globalThis`) exposes
 * `showSaveFilePicker` — the auto tier-selection's tier-1 gate. */
export function fsAccessAvailable(target: unknown = globalThis): boolean {
  return getShowSaveFilePicker(target) !== undefined;
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
): Promise<SaveSink> {
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
      throw new GosdCancelledError(
        "the save dialog was dismissed before anything was downloaded",
        { cause },
      );
    }
    throw new GosdSaveFailedError(
      `opening the save dialog for "${options.suggestedName}" failed`,
      { cause },
    );
  }

  const writable = await handle.createWritable();

  return {
    kind: "fs-access",
    writable,
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
