// Tier 3, the last resort: accumulate the whole patched image in memory,
// then hand it to the browser as a Blob download once — and only once —
// everything upstream (fetch, preconditions, every placeholder hash, the
// final whole-image hash) has already succeeded. Peak memory is roughly
// twice the image size for a brief moment (the accumulated chunks, then
// also the Blob copying them) — fine for a config image, not for anything
// enormous. All DOM access (Blob, URL, document) happens inside functions,
// never at module scope, so importing this module in Node — as the unit
// tests and the Node-only core API do — never throws.

import { GosdSaveFailedError } from "../errors.js";
import type { SaveSink } from "./types.js";

export interface CreateMemorySinkOptions {
  /** The filename offered to the browser's download prompt. */
  filename: string;
}

export function createMemorySink(options: CreateMemorySinkOptions): SaveSink {
  const chunks: Uint8Array[] = [];
  let aborted = false;

  const writable = new WritableStream<Uint8Array>({
    write(chunk) {
      chunks.push(chunk);
    },
  });

  return {
    kind: "memory",
    writable,
    async commit() {
      if (aborted) return;
      try {
        triggerBlobDownload(new Blob(chunks as BlobPart[]), options.filename);
      } catch (cause) {
        throw new GosdSaveFailedError(
          `saving "${options.filename}" failed while building the in-memory download; the memory tier needs a browser DOM (Blob, URL.createObjectURL, document) — see the package README's tier table`,
          { cause },
        );
      } finally {
        chunks.length = 0;
      }
    },
    async abort() {
      aborted = true;
      chunks.length = 0;
    },
  };
}

function triggerBlobDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.style.display = "none";
  document.body.appendChild(anchor);
  try {
    anchor.click();
  } finally {
    document.body.removeChild(anchor);
    // The browser has already been handed the object URL synchronously by
    // `click()`; this delay just gives it time to open/start the download
    // before the URL stops resolving.
    setTimeout(() => URL.revokeObjectURL(url), 30000);
  }
}
