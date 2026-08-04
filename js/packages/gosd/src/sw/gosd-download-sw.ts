// The service-worker half of the opt-in "service-worker" save tier (see
// ../downloads/sinks/service-worker.ts for the client half and the full
// protocol writeup). This file is compiled ALONE, via tsconfig.sw.json, to
// dist/sw/gosd-download-sw.js — a classic script with zero `import`/`export`
// statements, because module service workers aren't universally supported.
// The integrator hosts the compiled file at a same-origin URL and registers
// it themselves; this package never registers anything on its own.
//
// TypeScript's built-in "webworker" lib covers generic worker/fetch/stream
// globals but not the ServiceWorker-specific extensions (self.skipWaiting,
// self.clients, self.registration, install/activate/fetch event shapes) —
// there's no first-party lib for those and this package takes on no
// devDependencies to fill the gap, so the handful used here are hand-typed
// as local interfaces and reached via a single cast of `self`, rather than
// by re-declaring the ambient `self` global (which would conflict with the
// lib's own declaration).
//
// PROTOCOL must match the identical constant in
// ../downloads/sinks/service-worker.ts — service-worker.test.ts asserts the
// two source files agree.
const PROTOCOL = 1;

interface ExtendableEventLike extends Event {
  waitUntil(promise: Promise<unknown>): void;
}

interface FetchEventLike extends Event {
  request: Request;
  respondWith(response: Response | Promise<Response>): void;
}

interface ServiceWorkerSelfLike {
  skipWaiting(): Promise<void>;
  clients: { claim(): Promise<void> };
  registration: { scope: string };
}

const sw = self as unknown as ServiceWorkerSelfLike;

interface PendingEntry {
  filename: string;
  size: number;
  createdAt: number;
  fetched: boolean;
  readable: ReadableStream<Uint8Array>;
  writer: WritableStreamDefaultWriter<Uint8Array>;
}

// A download not fetched within this long of its "begin" message is
// presumed abandoned (e.g. the triggering iframe never loaded) and is
// garbage-collected rather than held forever.
const ENTRY_TTL_MS = 5 * 60 * 1000;

const pending = new Map<string, PendingEntry>();

function sweepExpiredEntries(): void {
  const now = Date.now();
  for (const [id, entry] of pending) {
    if (!entry.fetched && now - entry.createdAt > ENTRY_TTL_MS) {
      pending.delete(id);
      entry.writer.abort(new Error("gosd: download expired before being fetched")).catch(() => {});
    }
  }
}

self.addEventListener("install", (event) => {
  (event as unknown as ExtendableEventLike).waitUntil(sw.skipWaiting());
});

self.addEventListener("activate", (event) => {
  (event as unknown as ExtendableEventLike).waitUntil(sw.clients.claim());
});

interface BeginMessage {
  gosd?: number;
  type?: string;
  id?: string;
  filename?: string;
  size?: number;
}

self.addEventListener("message", (event: MessageEvent) => {
  const data = event.data as BeginMessage | undefined;
  if (!data || data.gosd !== PROTOCOL || data.type !== "begin") return;
  if (
    typeof data.id !== "string" ||
    typeof data.filename !== "string" ||
    typeof data.size !== "number"
  )
    return;

  const port = event.ports[0];
  if (!port) return;

  sweepExpiredEntries();

  const transform = new TransformStream<Uint8Array, Uint8Array>();
  const entry: PendingEntry = {
    filename: data.filename,
    size: data.size,
    createdAt: Date.now(),
    fetched: false,
    readable: transform.readable,
    writer: transform.writable.getWriter(),
  };
  pending.set(data.id, entry);

  port.onmessage = (portEvent: MessageEvent) => {
    handlePortMessage(entry, port, portEvent.data as PortMessage | undefined);
  };
  port.postMessage({ type: "ready", protocol: PROTOCOL });
});

interface PortMessage {
  type?: string;
  stream?: ReadableStream<Uint8Array>;
  chunk?: Uint8Array;
  reason?: unknown;
}

function handlePortMessage(
  entry: PendingEntry,
  port: MessagePort,
  data: PortMessage | undefined,
): void {
  if (!data) return;

  switch (data.type) {
    case "ping":
      port.postMessage({ type: "pong" });
      return;
    case "stream":
      if (data.stream) pumpTransferredStream(entry, data.stream);
      return;
    case "chunk":
      // Pump mode's backpressure: the page holds the next chunk until this
      // ack, sent only once the write lands (i.e. the download has drained
      // the previous one), so a slow disk never buffers the image here.
      if (data.chunk) {
        entry.writer
          .write(data.chunk)
          .then(() => port.postMessage({ type: "ack" }))
          .catch((err: unknown) => port.postMessage({ type: "error", reason: String(err) }));
      }
      return;
    case "end":
      entry.writer.close().catch(() => {});
      return;
    case "abort":
      entry.writer
        .abort(data.reason ?? new Error("gosd: download aborted by the page"))
        .catch(() => {});
      return;
    default:
      return;
  }
}

function pumpTransferredStream(entry: PendingEntry, stream: ReadableStream<Uint8Array>): void {
  const reader = stream.getReader();
  void (async () => {
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          await entry.writer.close();
          return;
        }
        await entry.writer.write(value);
      }
    } catch (err) {
      try {
        await entry.writer.abort(err);
      } catch {
        // Already closed/aborted; nothing more to do.
      }
    }
  })();
}

self.addEventListener("fetch", (event) => {
  const fe = event as unknown as FetchEventLike;
  if (fe.request.method !== "GET") return;

  const match = matchDownloadURL(fe.request.url);
  if (!match) return;

  fe.respondWith(handleDownloadFetch(match.id, match.filename));
});

function matchDownloadURL(href: string): { id: string; filename: string } | null {
  const prefix = `${sw.registration.scope}gosd/`;
  if (!href.startsWith(prefix)) return null;
  const rest = href.slice(prefix.length);
  const slash = rest.indexOf("/");
  if (slash === -1) return null;
  return {
    id: decodeURIComponent(rest.slice(0, slash)),
    filename: decodeURIComponent(rest.slice(slash + 1)),
  };
}

async function handleDownloadFetch(id: string, filename: string): Promise<Response> {
  const entry = pending.get(id);
  if (!entry || entry.filename !== filename) {
    return new Response(
      "gosd: no pending download for this URL (expired, already fetched, or never registered)",
      {
        status: 404,
      },
    );
  }
  pending.delete(id);
  entry.fetched = true;

  return new Response(entry.readable, {
    headers: {
      "Content-Type": "application/octet-stream",
      "Content-Disposition": `attachment; filename*=UTF-8''${rfc5987Encode(entry.filename)}`,
      "Content-Length": String(entry.size),
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
    },
  });
}

/** RFC 5987 `ext-value` percent-encoding: `encodeURIComponent` already
 * escapes everything outside the RFC's `attr-char` set except `! ' ( ) *`,
 * so those five are escaped by hand on top of it. */
function rfc5987Encode(value: string): string {
  return encodeURIComponent(value).replace(
    /[!'()*]/g,
    (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`,
  );
}
