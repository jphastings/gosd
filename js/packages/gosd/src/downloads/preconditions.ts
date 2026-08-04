// Fail-fast checks against the image response's headers, run once before a
// single body byte is piped anywhere. The download host is untrusted (only
// the manifest is), so these are optimizations for catching an obviously
// wrong response early — never a substitute for the full streamed SHA-256
// substitute.ts always performs. A header match here NEVER skips that hash.

import { GosdImagePreconditionError } from "./errors.js";
import type { Manifest } from "./manifest.js";

const HEX64 = /^[0-9a-f]{64}$/i;

export interface CheckImageResponseOptions {
  /** Skips the ETag check entirely. The Content-Length check still runs
   * (disable it per-request by having the server omit that header, or by
   * serving with a Content-Encoding). Not recommended — see the ETag
   * policy in the package README. */
  ignoreETag?: boolean;
}

/** Checks `response`'s `ETag` and `Content-Length` headers against
 * `manifest.image` before anything is read from its body.
 *
 * - A 64-hex-character ETag (after stripping a leading `W/` and
 *   surrounding quotes) that disagrees with `image.sha256` throws
 *   `GosdImagePreconditionError`. An ETag that isn't a bare 64-hex string
 *   (a weak validator, a quoted opaque token, absent entirely) is ignored —
 *   it isn't a sha256 to compare against.
 * - A `Content-Length` that disagrees with `image.size` throws the same
 *   error, unless `Content-Encoding` is present (the header would then
 *   describe the encoded length, not the decoded image size). */
export function checkImageResponse(
  response: Response,
  manifest: Manifest,
  options: CheckImageResponseOptions = {},
): void {
  if (!options.ignoreETag) {
    const etagHeader = response.headers.get("etag");
    if (etagHeader !== null) {
      const stripped = etagHeader.replace(/^W\//, "").replace(/^"(.*)"$/, "$1");
      if (
        HEX64.test(stripped) &&
        stripped.toLowerCase() !== manifest.image.sha256.toLowerCase()
      ) {
        throw new GosdImagePreconditionError(
          `the image response's ETag (${stripped}) does not match the manifest's expected sha256 (${manifest.image.sha256}); the CDN may be serving stale or mismatched content for this URL. Pass options.ignoreETag to bypass this check (not recommended — the full download is still hashed either way)`,
        );
      }
    }
  }

  if (response.headers.get("content-encoding") === null) {
    const contentLengthHeader = response.headers.get("content-length");
    if (contentLengthHeader !== null) {
      const contentLength = Number(contentLengthHeader);
      if (
        Number.isFinite(contentLength) &&
        contentLength !== manifest.image.size
      ) {
        throw new GosdImagePreconditionError(
          `the image response's Content-Length (${contentLengthHeader}) does not match the manifest's declared image size (${manifest.image.size}); the download would end up truncated or overlong`,
        );
      }
    }
  }
}
