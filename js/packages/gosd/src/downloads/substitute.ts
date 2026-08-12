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
//
// Steps 1-4 are implemented as a plain, streamless "engine" below
// (createEngine/engineProcessChunk/engineFinish) so the exact same logic
// backs two different callers: the live TransformStream, and
// `primeSubstitutionState`, which replays an already-downloaded (and, for
// any patched placeholder within it, pristine-reconstructed — see
// resume.ts) prefix through it to rebuild the running hash state a resumed
// download needs to continue correctly, without a serialized-hasher-state
// format to maintain (the vendored Sha256 only supports in-session
// `clone()`; re-hashing a reconstructed prefix is the cross-session
// equivalent).

import {
  GosdImageHashMismatchError,
  GosdImageSizeError,
  GosdPlaceholderNotPristineError,
} from "./errors.js";
import type { Manifest } from "./manifest.js";
import { injectableRegions } from "./manifest.js";
import { Sha256 } from "./sha256.js";

export interface SubstitutionProgress {
  bytesProcessed: number;
  bytesTotal: number;
}

export interface SubstitutionOptions {
  onProgress?: (progress: SubstitutionProgress) => void;
  /** Fires the instant a region that `padded` supplied a replacement for
   * finishes verifying, with that region's PRISTINE (pre-substitution)
   * bytes — never for a region `padded` left untouched (its on-disk bytes
   * already are its pristine bytes). Used by the fs-access resumable
   * download path (resume.ts) to stash those bytes for reconstructing this
   * prefix again in a later session. The key is a placeholder's path, or
   * the reserved config file's path. */
  onPlaceholderVerified?: (key: string, pristine: Uint8Array) => void;
}

interface Segment {
  start: number;
  end: number;
  regionIndex: number;
  /** The full replacement content for this segment (same length as
   * `end - start`), or null when this region was left untouched. */
  replacement: Uint8Array | null;
}

interface RegionState {
  key: string;
  label: string;
  sha256: string;
  hasher: Sha256;
  remaining: number;
  /** Accumulates this region's pristine bytes as they're seen, but only
   * when it's being patched (`onPlaceholderVerified` needs them for a
   * patched region; an untouched one's on-disk bytes are already its
   * pristine bytes, so capturing them again would just waste memory). */
  capture: Uint8Array | null;
  captureOffset: number;
}

/** The running state of one substitution pass: how far through the image
 * it's gotten, the whole-image hash so far, and each placeholder's own
 * in-progress hash. Produced by `primeSubstitutionState` and consumed by
 * `createSubstitutionTransform`'s `resumeFrom` to continue a download
 * across a browser session (see resume.ts) — opaque otherwise, and never
 * serialized (a fresh prime from a reconstructed prefix stands in for
 * serialization; see the module doc above). */
export interface SubstitutionState {
  pos: number;
  segIndex: number;
  imageHasher: Sha256;
  placeholderStates: RegionState[];
}

interface Engine {
  manifest: Manifest;
  segments: Segment[];
  options: SubstitutionOptions;
  state: SubstitutionState;
}

function buildSegments(manifest: Manifest, padded: Map<string, Uint8Array>): Segment[] {
  const segments: Segment[] = [];
  injectableRegions(manifest).forEach((region, regionIndex) => {
    const replacement = padded.get(region.key);
    let consumed = 0;
    for (const r of region.ranges) {
      segments.push({
        start: r.offset,
        end: r.offset + r.length,
        regionIndex,
        replacement: replacement ? replacement.subarray(consumed, consumed + r.length) : null,
      });
      consumed += r.length;
    }
  });
  segments.sort((a, b) => a.start - b.start);
  return segments;
}

function freshState(manifest: Manifest, padded: Map<string, Uint8Array>): SubstitutionState {
  return {
    pos: 0,
    segIndex: 0,
    imageHasher: new Sha256(),
    placeholderStates: injectableRegions(manifest).map((region) => ({
      key: region.key,
      label: region.label,
      sha256: region.sha256,
      hasher: new Sha256(),
      remaining: region.size,
      capture: padded.has(region.key) ? new Uint8Array(region.size) : null,
      captureOffset: 0,
    })),
  };
}

function createEngine(
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  options: SubstitutionOptions,
  resumeFrom?: SubstitutionState,
): Engine {
  return {
    manifest,
    segments: buildSegments(manifest, padded),
    options,
    state: resumeFrom ?? freshState(manifest, padded),
  };
}

function verifyPristine(engine: Engine, state: RegionState): void {
  const digest = state.hasher.digestHex();
  if (digest !== state.sha256) {
    throw new GosdPlaceholderNotPristineError(
      `${state.label} is not pristine: expected sha256 ${state.sha256}, got ${digest}; the image may be corrupt, already patched by something else, or tampered with`,
    );
  }
  if (state.capture) {
    engine.options.onPlaceholderVerified?.(state.key, state.capture);
  }
}

/** Runs the engine's start-of-stream check: a zero-size placeholder never
 * has a byte cross the loop in `engineProcessChunk`, so it's verified up
 * front against the empty-input digest. Skipped when resuming (a resumed
 * engine's placeholders were already checked during priming). */
function engineStart(engine: Engine): void {
  for (const state of engine.state.placeholderStates) {
    if (state.remaining === 0) {
      verifyPristine(engine, state);
    }
  }
}

/** Feeds one chunk through the engine, returning the bytes to emit
 * (possibly rewritten with replacement content, possibly the same chunk
 * unchanged). Shared, verbatim, between the live TransformStream and
 * `primeSubstitutionState`'s replay of an already-downloaded prefix. */
function engineProcessChunk(engine: Engine, chunk: Uint8Array): Uint8Array {
  const { state, segments, manifest } = engine;
  const chunkStart = state.pos;
  const chunkEnd = state.pos + chunk.length;

  if (chunkEnd > manifest.image.size) {
    throw new GosdImageSizeError(
      `the downloaded image exceeds its manifest's declared size of ${manifest.image.size} bytes; the download or the manifest is corrupt`,
    );
  }

  state.imageHasher.update(chunk);

  let out: Uint8Array | null = null;

  while (state.segIndex < segments.length) {
    const seg = segments[state.segIndex];
    if (!seg || seg.start >= chunkEnd) break;

    const intersectStart = Math.max(seg.start, chunkStart);
    const intersectEnd = Math.min(seg.end, chunkEnd);

    if (intersectEnd > intersectStart) {
      const localStart = intersectStart - chunkStart;
      const localEnd = intersectEnd - chunkStart;
      const placeholderState = state.placeholderStates[seg.regionIndex];
      if (!placeholderState) {
        throw new Error("substitute: internal error, segment references an unknown region");
      }

      const original = chunk.subarray(localStart, localEnd);
      placeholderState.hasher.update(original);
      if (placeholderState.capture) {
        placeholderState.capture.set(original, placeholderState.captureOffset);
        placeholderState.captureOffset += original.length;
      }
      placeholderState.remaining -= intersectEnd - intersectStart;
      if (placeholderState.remaining === 0) {
        verifyPristine(engine, placeholderState);
      }

      if (seg.replacement) {
        if (!out) out = chunk.slice();
        out.set(
          seg.replacement.subarray(intersectStart - seg.start, intersectEnd - seg.start),
          localStart,
        );
      }
    }

    if (seg.end <= chunkEnd) {
      state.segIndex++;
    } else {
      break;
    }
  }

  state.pos = chunkEnd;
  engine.options.onProgress?.({ bytesProcessed: state.pos, bytesTotal: manifest.image.size });

  return out ?? chunk;
}

function engineFinish(engine: Engine): void {
  const { state, manifest } = engine;
  if (state.pos !== manifest.image.size) {
    throw new GosdImageSizeError(
      `the download ended after ${state.pos} bytes but the manifest declares the image as ${manifest.image.size} bytes; the download was interrupted, or the manifest is stale`,
    );
  }
  const digest = state.imageHasher.digestHex();
  if (digest !== manifest.image.sha256) {
    throw new GosdImageHashMismatchError(
      `the downloaded image failed final integrity verification: expected sha256 ${manifest.image.sha256}, got ${digest}; the download is corrupt, or the image at that URL has changed since the manifest was written`,
    );
  }
}

/** Replays `prefix` — the reconstructed-pristine bytes of an
 * already-downloaded prefix of the image (see resume.ts's
 * `reconstructPristinePrefix`) — through the same verification/hashing
 * logic a live download uses, without writing the result anywhere.
 * Returns the resulting `SubstitutionState`, ready to hand to
 * `createSubstitutionTransform`'s `resumeFrom` to continue the download
 * live from `prefix.length` onward. Throws exactly as a live download
 * would if `prefix` doesn't actually verify (e.g. it wasn't correctly
 * reconstructed, or the on-disk file was corrupted) — the caller should
 * treat that as "this resume isn't safe; discard it and start fresh". */
export function primeSubstitutionState(
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  prefix: Uint8Array,
): SubstitutionState {
  const engine = createEngine(manifest, padded, {});
  engineStart(engine);
  if (prefix.length > 0) {
    engineProcessChunk(engine, prefix);
  }
  return engine.state;
}

/** Builds the substitution TransformStream for one download: feed it the
 * raw image bytes in order, and it emits the same bytes with every
 * `padded` placeholder's ranges rewritten in place, throwing as soon as
 * anything fails verification. `padded` (from `padContents`) need not
 * cover every placeholder — the rest are verified but left untouched.
 *
 * `resumeFrom` (from `primeSubstitutionState`) starts the transform partway
 * through the image instead of at byte 0, continuing an already-verified
 * prefix's running hashes — the caller is responsible for only ever
 * feeding this transform the bytes that come immediately after that
 * prefix. */
export function createSubstitutionTransform(
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  options: SubstitutionOptions = {},
  resumeFrom?: SubstitutionState,
): TransformStream<Uint8Array, Uint8Array> {
  const engine = createEngine(manifest, padded, options, resumeFrom);

  return new TransformStream<Uint8Array, Uint8Array>({
    start() {
      if (!resumeFrom) engineStart(engine);
    },
    transform(chunk, controller) {
      controller.enqueue(engineProcessChunk(engine, chunk));
    },
    flush() {
      engineFinish(engine);
    },
  });
}

/** Convenience wrapper: pipes `source` through `createSubstitutionTransform`. */
export function patchStream(
  source: ReadableStream<Uint8Array>,
  manifest: Manifest,
  padded: Map<string, Uint8Array>,
  options: SubstitutionOptions = {},
  resumeFrom?: SubstitutionState,
): ReadableStream<Uint8Array> {
  return source.pipeThrough(createSubstitutionTransform(manifest, padded, options, resumeFrom));
}
