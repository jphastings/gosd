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
