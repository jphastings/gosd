// Orchestrates one download: fetch the image (via a caller-supplied
// provider, so this module doesn't need to know about URLs or `fetch`
// options), check its response preconditions, pipe it through the
// substitution transform into a save sink, and commit — aborting the sink
// on any failure along the way so a partial/corrupt result is never left
// looking complete. Tier selection and sink construction happen in
// index.ts, before this function is ever called (see that module's ordering
// requirement for the fs-access tier).

import { GosdError, GosdImageFetchError } from "./errors.js";
import { checkImageResponse } from "./preconditions.js";
import type { Manifest } from "./manifest.js";
import type { SaveSink } from "./sinks/types.js";
import { patchStream, type SubstitutionProgress } from "./substitute.js";

/** Produces the image `Response`, however the caller wants to fetch it
 * (URL, headers, `fetch` override) — `runDownload` only needs the result. */
export type ImageResponseProvider = () => Promise<Response>;

export interface RunDownloadOptions {
  manifest: Manifest;
  padded: Map<string, Uint8Array>;
  fetchImage: ImageResponseProvider;
  sink: SaveSink;
  ignoreETag?: boolean;
  signal?: AbortSignal;
  onProgress?: (progress: SubstitutionProgress) => void;
}

export interface RunDownloadResult {
  sha256: string;
}

/** Runs one download end to end against an already-created sink and an
 * already-fetched-or-fetchable manifest. On any failure — fetch, precondition,
 * substitution/verification, or the sink itself — the sink is aborted before
 * the error is rethrown; `sink.commit()` is only ever called after the patched
 * stream has fully and successfully piped into it. */
export async function runDownload(
  options: RunDownloadOptions,
): Promise<RunDownloadResult> {
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
      { onProgress: options.onProgress },
    );
    await patched.pipeTo(options.sink.writable, { signal: options.signal });
  } catch (err) {
    await options.sink.abort(err);
    throw err;
  }

  await options.sink.commit();
  return { sha256: options.manifest.image.sha256 };
}
