// The heart of withPlaceholders: a TransformStream that, as image bytes
// flow past, verifies the whole image's SHA-256, verifies each
// placeholder's currently-pristine bytes against its manifest hash the
// moment they're fully seen, and — for placeholders the caller supplied
// content for — rewrites those ranges to the padded replacement bytes, all
// without ever buffering more than the current chunk (copy-on-write, only
// for chunks a replacement actually touches).
//
// Algorithm (see docs/image-injection.md for the byte-range contract this
// consumes):
//
//   1. Flatten every placeholder's ranges (patched and untouched alike)
//      into one globally sorted list of segments. A patched range's
//      replacement bytes are sliced from that placeholder's padded content
//      by walking its ranges in order with a running offset — this is what
//      keeps a fragmented placeholder (several ranges) correct.
//   2. Per chunk: reject overflow past image.size before hashing or
//      enqueueing anything; hash the chunk's ORIGINAL bytes into the whole-
//      image hash; walk every segment intersecting this chunk, feeding each
//      one's original bytes into its own placeholder hash and copy-on-write
//      substituting replacement bytes where one exists; enqueue the
//      (possibly rewritten) chunk.
//   3. The instant a placeholder's last byte is seen, verify its digest
//      immediately (mid-stream — don't wait for flush) so a tampered or
//      already-patched placeholder aborts as early as possible.
//   4. At flush: a short stream (never reached image.size) or the final
//      whole-image digest mismatching aborts the stream.
//
// A throw from `transform`/`flush` errors the TransformStream, which in a
// `pipeTo` chain cancels the upstream fetch and aborts the downstream sink
// — one error channel for the whole pipeline.

import {
  GosdImageHashMismatchError,
  GosdImageSizeError,
  GosdPlaceholderNotPristineError,
} from "./errors.js";
import type { Manifest } from "./manifest.js";
import { Sha256 } from "./sha256.js";

export interface SubstitutionProgress {
  bytesProcessed: number;
  bytesTotal: number;
}

export interface SubstitutionOptions {
  onProgress?: (progress: SubstitutionProgress) => void;
}

interface Segment {
  start: number;
  end: number;
  placeholderIndex: number;
  /** The full replacement content for this segment (same length as
   * `end - start`), or null when this placeholder was left untouched. */
  replacement: Uint8Array | null;
}

interface PlaceholderState {
  path: string;
  sha256: string;
  hasher: Sha256;
  remaining: number;
}

function buildPlan(
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
): { segments: Segment[]; placeholderStates: PlaceholderState[] } {
  const placeholderStates: PlaceholderState[] = manifest.placeholders.map(
    (p) => ({
      path: p.path,
      sha256: p.sha256,
      hasher: new Sha256(),
      remaining: p.size,
    }),
  );

  const segments: Segment[] = [];
  manifest.placeholders.forEach((p, placeholderIndex) => {
    const replacement = padded.get(p.path);
    let consumed = 0;
    for (const r of p.ranges) {
      segments.push({
        start: r.offset,
        end: r.offset + r.length,
        placeholderIndex,
        replacement: replacement
          ? replacement.subarray(consumed, consumed + r.length)
          : null,
      });
      consumed += r.length;
    }
  });
  segments.sort((a, b) => a.start - b.start);

  return { segments, placeholderStates };
}

function verifyPristine(state: PlaceholderState): void {
  const digest = state.hasher.digestHex();
  if (digest !== state.sha256) {
    throw new GosdPlaceholderNotPristineError(
      `placeholder "${state.path}" is not pristine: expected sha256 ${state.sha256}, got ${digest}; the image may be corrupt, already patched by something else, or tampered with`,
    );
  }
}

/** Builds the substitution TransformStream for one download: feed it the
 * raw image bytes in order, and it emits the same bytes with every
 * `padded` placeholder's ranges rewritten in place, throwing as soon as
 * anything fails verification. `padded` (from `padContents`) need not
 * cover every placeholder — the rest are verified but left untouched. */
export function createSubstitutionTransform(
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  options: SubstitutionOptions = {},
): TransformStream<Uint8Array, Uint8Array> {
  const { segments, placeholderStates } = buildPlan(manifest, padded);
  const imageHasher = new Sha256();
  const imageSize = manifest.image.size;

  let pos = 0;
  let segIndex = 0;

  return new TransformStream<Uint8Array, Uint8Array>({
    start() {
      // A zero-size placeholder (degenerate, but not forbidden by the
      // manifest schema) never has a byte cross the loop below, so verify
      // it up front against the empty-input digest.
      for (const state of placeholderStates) {
        if (state.remaining === 0) {
          verifyPristine(state);
        }
      }
    },

    transform(chunk, controller) {
      const chunkStart = pos;
      const chunkEnd = pos + chunk.length;

      if (chunkEnd > imageSize) {
        throw new GosdImageSizeError(
          `the downloaded image exceeds its manifest's declared size of ${imageSize} bytes; the download or the manifest is corrupt`,
        );
      }

      imageHasher.update(chunk);

      let out: Uint8Array | null = null;

      while (segIndex < segments.length) {
        const seg = segments[segIndex];
        if (!seg || seg.start >= chunkEnd) break;

        const intersectStart = Math.max(seg.start, chunkStart);
        const intersectEnd = Math.min(seg.end, chunkEnd);

        if (intersectEnd > intersectStart) {
          const localStart = intersectStart - chunkStart;
          const localEnd = intersectEnd - chunkStart;
          const state = placeholderStates[seg.placeholderIndex];
          if (!state)
            throw new Error(
              "substitute: internal error, segment references an unknown placeholder",
            );

          state.hasher.update(chunk.subarray(localStart, localEnd));
          state.remaining -= intersectEnd - intersectStart;
          if (state.remaining === 0) {
            verifyPristine(state);
          }

          if (seg.replacement) {
            if (!out) out = chunk.slice();
            out.set(
              seg.replacement.subarray(
                intersectStart - seg.start,
                intersectEnd - seg.start,
              ),
              localStart,
            );
          }
        }

        if (seg.end <= chunkEnd) {
          segIndex++;
        } else {
          break;
        }
      }

      controller.enqueue(out ?? chunk);
      pos = chunkEnd;
      options.onProgress?.({ bytesProcessed: pos, bytesTotal: imageSize });
    },

    flush() {
      if (pos !== imageSize) {
        throw new GosdImageSizeError(
          `the download ended after ${pos} bytes but the manifest declares the image as ${imageSize} bytes; the download was interrupted, or the manifest is stale`,
        );
      }
      const digest = imageHasher.digestHex();
      if (digest !== manifest.image.sha256) {
        throw new GosdImageHashMismatchError(
          `the downloaded image failed final integrity verification: expected sha256 ${manifest.image.sha256}, got ${digest}; the download is corrupt, or the image at that URL has changed since the manifest was written`,
        );
      }
    },
  });
}

/** Convenience wrapper: pipes `source` through `createSubstitutionTransform`. */
export function patchStream(
  source: ReadableStream<Uint8Array>,
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  options: SubstitutionOptions = {},
): ReadableStream<Uint8Array> {
  return source.pipeThrough(
    createSubstitutionTransform(manifest, padded, options),
  );
}
