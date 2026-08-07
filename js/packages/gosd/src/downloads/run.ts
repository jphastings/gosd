// Orchestrates one download: fetch the image (via a caller-supplied
// provider, so this module doesn't need to know about URLs or `fetch`
// options), check its response preconditions, pipe it through the
// substitution transform into a save sink, and commit — aborting the sink
// on any failure along the way so a partial/corrupt result is never left
// looking complete. Tier selection and sink construction happen in
// index.ts, before this function is ever called (see that module's ordering
// requirement for the fs-access tier).
//
// `checkpoint` (optional; used only by the fs-access resumable path, see
// resume.ts) changes exactly one thing about failure handling: instead of
// aborting (discarding) the sink on a failure the checkpoint doesn't
// consider untrustworthy, the sink is committed as-is, preserving whatever
// was durably streamed so far for a later `resumeDownload` — the failure is
// still rethrown either way, so a caller not using resume.ts sees no
// difference in outcome, only in what's left on disk afterward.

import {
  GosdError,
  GosdImageFetchError,
  GosdImageHashMismatchError,
  GosdImageSizeError,
  GosdPlaceholderNotPristineError,
} from "./errors.js";
import { checkImageResponse } from "./preconditions.js";
import type { Manifest } from "./manifest.js";
import type { SaveSink } from "./sinks/types.js";
import { patchStream, type SubstitutionProgress, type SubstitutionState } from "./substitute.js";

/** Produces the image `Response`, however the caller wants to fetch it
 * (URL, headers, `fetch` override) — `runDownload` only needs the result. */
export type ImageResponseProvider = () => Promise<Response>;

/** Hooks a resumable download's checkpointing into `runDownload`, without
 * `runDownload` itself knowing anything about IndexedDB or the fs-access
 * tier (see resume.ts, which builds these). Every hook is called
 * best-effort from `runDownload`'s perspective: none may throw (persist
 * failures should be swallowed where they're implemented — resumability is
 * an optimization, not a correctness requirement). */
export interface DownloadCheckpoint {
  /** Called once the image response's headers are known, before any body
   * bytes are streamed — used to capture the ETag/Last-Modified a future
   * resume's `If-Range` needs. */
  onResponseHeaders?: (headers: { etag: string | null; lastModified: string | null }) => void;
  /** Called the instant a patched placeholder finishes verifying, with its
   * pristine (pre-substitution) bytes — see
   * `SubstitutionOptions.onPlaceholderVerified`. */
  onPlaceholderVerified?: (path: string, pristine: Uint8Array) => void;
  /** Called once this attempt is done with the sink: `completed: true`
   * after a normal, full success; `completed: false` after a recoverable
   * failure whose partial bytes were preserved (see `isUntrustworthy`).
   * Never called when the sink was aborted instead. */
  onFinalized?: (completed: boolean) => void | Promise<void>;
  /** True for an error that means the bytes streamed so far can't be
   * trusted and must be discarded (abort) rather than preserved for a
   * future resume (commit). Defaults to classifying the substitution
   * transform's own verification failures this way; anything else
   * (network drops, aborts, a non-ok response) is treated as recoverable. */
  isUntrustworthy?: (err: unknown) => boolean;
}

export interface RunDownloadOptions {
  manifest: Manifest;
  padded: Map<string, Uint8Array>;
  fetchImage: ImageResponseProvider;
  sink: SaveSink;
  ignoreETag?: boolean;
  signal?: AbortSignal;
  onProgress?: (progress: SubstitutionProgress) => void;
  /** Continues a substitution pass already primed up to some byte offset
   * (see `primeSubstitutionState`) instead of starting fresh from byte 0 —
   * the caller must ensure `fetchImage`'s response body starts at exactly
   * that offset (e.g. via a `Range` request). */
  resumeFrom?: SubstitutionState;
  checkpoint?: DownloadCheckpoint;
}

export interface RunDownloadResult {
  sha256: string;
}

function defaultIsUntrustworthy(err: unknown): boolean {
  return (
    err instanceof GosdPlaceholderNotPristineError ||
    err instanceof GosdImageHashMismatchError ||
    err instanceof GosdImageSizeError
  );
}

/** Runs one download end to end against an already-created sink and an
 * already-fetched-or-fetchable manifest. On any failure — fetch, precondition,
 * substitution/verification, or the sink itself — the sink is aborted before
 * the error is rethrown (or, with `checkpoint` set and the failure not
 * classified as untrustworthy, committed instead — see the module doc);
 * `sink.commit()` is otherwise only ever called after the patched stream has
 * fully and successfully piped into it. */
export async function runDownload(options: RunDownloadOptions): Promise<RunDownloadResult> {
  try {
    let response: Response;
    try {
      response = await options.fetchImage();
    } catch (cause) {
      if (cause instanceof GosdError) throw cause;
      throw new GosdImageFetchError(
        "fetching the image failed because the request could not be completed; check network connectivity and the image URL",
        { cause },
      );
    }
    if (!response.ok) {
      throw new GosdImageFetchError(
        `fetching the image failed: the server responded with HTTP ${response.status}`,
      );
    }

    checkImageResponse(response, options.manifest, {
      ignoreETag: options.ignoreETag,
      expectedContentLength: options.manifest.image.size - (options.resumeFrom?.pos ?? 0),
    });

    options.checkpoint?.onResponseHeaders?.({
      etag: response.headers.get("etag"),
      lastModified: response.headers.get("last-modified"),
    });

    if (!response.body) {
      throw new GosdImageFetchError(
        "fetching the image failed: the response has no readable body to stream",
      );
    }

    const patched = patchStream(
      response.body,
      options.manifest,
      options.padded,
      {
        onProgress: options.onProgress,
        onPlaceholderVerified: options.checkpoint?.onPlaceholderVerified,
      },
      options.resumeFrom,
    );
    await patched.pipeTo(options.sink.writable, {
      signal: options.signal,
      preventAbort: options.checkpoint !== undefined,
    });
  } catch (err) {
    const checkpoint = options.checkpoint;
    if (checkpoint && !(checkpoint.isUntrustworthy ?? defaultIsUntrustworthy)(err)) {
      await options.sink.commit();
      await checkpoint.onFinalized?.(false);
    } else {
      await options.sink.abort(err);
    }
    throw err;
  }

  await options.sink.commit();
  await options.checkpoint?.onFinalized?.(true);
  return { sha256: options.manifest.image.sha256 };
}
