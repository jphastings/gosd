// The save-tier abstraction: whatever writes the patched image to its
// final destination implements this. Sequential-only by design (a plain
// WritableStream, written once, start to finish) — download resuming is a
// follow-up that would extend this interface, not replace it.

export type SaveSinkKind = "fs-access" | "service-worker" | "memory";

export interface SaveSink {
  readonly kind: SaveSinkKind;
  /** Written to exactly once, start to finish, by `runDownload`'s
   * `pipeTo`. */
  readonly writable: WritableStream<Uint8Array>;
  /** Called after `writable` has been fully and successfully written to.
   * Finalizes the save (e.g. closes a file handle, triggers a browser
   * download). */
  commit(): Promise<void>;
  /** Called instead of `commit()` on any failure anywhere in the download
   * (fetch, precondition, verification, or a write itself) — must leave no
   * misleadingly-complete output behind. `reason` is the error that
   * triggered the abort. */
  abort(reason: unknown): Promise<void>;
}

/** An optional capability a SaveSink MAY additionally implement, rather
 * than a change to the base (deliberately sequential-only) contract above.
 * Only the fs-access tier does today — resuming a download needs a real,
 * reopenable destination to read back and continue writing to; the memory
 * and service-worker tiers have nothing durable to resume from across a
 * page reload. See resume.ts. */
export interface SeekableSaveSink extends SaveSink {
  /** Reads back the bytes already durably written at this sink's
   * destination, e.g. left over from an earlier, interrupted attempt —
   * used to re-verify a partial download before trusting and resuming it. */
  readExisting(): Promise<Uint8Array>;
  /** An opaque, structured-clonable handle identifying this sink's
   * destination, suitable for persisting (e.g. in IndexedDB) and passing
   * back to this tier's own "reopen for resume" function in a later
   * session. Meaningful only to the tier that produced it. */
  readonly resumeHandle: unknown;
}

export function isSeekable(sink: SaveSink): sink is SeekableSaveSink {
  const candidate = sink as Partial<SeekableSaveSink>;
  return typeof candidate.readExisting === "function" && "resumeHandle" in candidate;
}
