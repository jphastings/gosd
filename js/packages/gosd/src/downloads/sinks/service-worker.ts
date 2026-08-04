// Tier 2 (opt-in): stream the patched image to a same-origin service worker
// so the browser shows a real download-manager entry with progress, without
// holding the whole image in memory. The integrator hosts the worker script
// this package ships at the `gosd/downloads/service-worker.js` export
// subpath (see package.json's exports map) at a same-origin URL and passes
// `options.serviceWorker = { url, scope }` — see the package README's "SW
// hosting" section.
//
// Handshake: a dedicated MessageChannel per download. The client posts
// `{gosd:1, type:"begin", id, filename, size}` to the active service
// worker's controller with the channel's port2 transferred; the worker
// replies `{type:"ready", protocol:PROTOCOL}` on that port once it has a
// pending entry for `id`. Body handoff then takes whichever path this
// browser supports:
//   - transferable ReadableStream (Chrome, Firefox): the client transfers a
//     TransformStream's readable half over the port and simply returns its
//     writable half as this sink's `writable` — pipeTo writes go straight
//     through to the worker with no extra copying, and pipeTo's own
//     close()/abort() on success/failure propagates to the transferred
//     stream automatically (Streams spec linkage), so the worker sees the
//     download end or fail with no separate message needed.
//   - pump mode (Safari, no transferable streams): the client instead
//     posts each chunk as `{type:"chunk", chunk}` (buffer transferred) and
//     waits for the worker's `{type:"ack"}` — sent only after the worker's
//     own write settles — before letting the next write proceed, then
//     `{type:"end"}` or `{type:"abort", reason}`. The one-chunk-in-flight
//     ack is the tier's backpressure: without it a download draining
//     slower than the network buffers the whole image inside the worker.
//     A missing ack doubles as the dead-worker watchdog.
// Either way the worker turns the same-origin GET
// `<scope>gosd/<id>/<filename>` (triggered here via a hidden iframe) into
// a `Response` streaming from whichever body it received.
//
// PROTOCOL must match the identical constant in ../../sw/gosd-download-sw.ts
// — service-worker.test.ts asserts the two source files agree, since the
// worker script (a classic, import-free script) can't share this module.

import { GosdSaveFailedError } from "../errors.js";
import type { SaveSink } from "./types.js";

/** Bump together with the same constant in gosd-download-sw.ts on any
 * incompatible wire-format change. */
export const PROTOCOL = 1;

const READY_TIMEOUT_MS = 60_000;
const PING_INTERVAL_MS = 20_000;
const IFRAME_CLEANUP_MS = 60_000;
const ACK_TIMEOUT_MS = 60_000;

export interface ServiceWorkerLocation {
  /** Same-origin URL of the worker script this package ships at the
   * `gosd/downloads/service-worker.js` export subpath. */
  url: string;
  scope: string;
}

export interface CreateServiceWorkerSinkOptions {
  filename: string;
  size: number;
  serviceWorker: ServiceWorkerLocation;
}

interface MinimalServiceWorkerContainer {
  controller: {
    postMessage(message: unknown, transfer: Transferable[]): void;
  } | null;
  register(
    url: string,
    options?: { scope?: string },
  ): Promise<{ active: unknown }>;
  ready: Promise<{ active: unknown }>;
}

function getServiceWorkerContainer(
  nav: unknown,
): MinimalServiceWorkerContainer | undefined {
  const sw = (nav as { serviceWorker?: unknown } | undefined)?.serviceWorker;
  return sw as MinimalServiceWorkerContainer | undefined;
}

/** True once a service worker is registered and active at `location`'s
 * scope — the auto tier-selection's tier-2 preflight. Safe to call
 * speculatively: registering an already-registered, unchanged worker URL
 * is a cheap no-op. */
export async function serviceWorkerAvailable(
  location: ServiceWorkerLocation,
  nav: unknown = typeof navigator === "undefined" ? undefined : navigator,
): Promise<boolean> {
  const sw = getServiceWorkerContainer(nav);
  if (!sw) return false;
  try {
    await sw.register(location.url, { scope: location.scope });
    const registration = await sw.ready;
    return registration.active !== null;
  } catch {
    return false;
  }
}

function randomId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** Detects transferable-ReadableStream support (Chrome, Firefox; not
 * Safari as of this writing) by actually attempting a transfer on a scratch
 * MessageChannel — the authoritative test, since feature flags drift. */
function canTransferReadableStream(): boolean {
  if (
    typeof MessageChannel === "undefined" ||
    typeof ReadableStream === "undefined"
  )
    return false;
  const { port1, port2 } = new MessageChannel();
  try {
    const probe = new ReadableStream();
    port1.postMessage(probe, [probe as unknown as Transferable]);
    return true;
  } catch {
    return false;
  } finally {
    port1.close();
    port2.close();
  }
}

/** Builds the pump-mode WritableStream over an established protocol port:
 * each write posts its chunk (buffer transferred) and resolves only on the
 * worker's `{type:"ack"}`, keeping exactly one chunk in flight — the
 * backpressure that stops a slow download from buffering the whole image
 * inside the worker. No ack within `ackTimeoutMs` fails the write, which
 * doubles as the dead-worker watchdog. Exported for its own unit tests;
 * `createServiceWorkerSink` is the real entry point. */
export function createPumpWritable(
  port: MessagePort,
  filename: string,
  ackTimeoutMs: number = ACK_TIMEOUT_MS,
): WritableStream<Uint8Array> {
  let pendingAck: {
    resolve: () => void;
    reject: (err: unknown) => void;
  } | null = null;

  port.onmessage = (event: MessageEvent) => {
    const data = event.data as { type?: string; reason?: unknown } | undefined;
    if (data?.type === "ack") {
      pendingAck?.resolve();
      pendingAck = null;
    } else if (data?.type === "error") {
      pendingAck?.reject(
        new GosdSaveFailedError(
          `the service worker failed to accept a chunk of "${filename}": ${String(data.reason)}`,
        ),
      );
      pendingAck = null;
    }
  };

  const settlePending = (reason: unknown) => {
    pendingAck?.reject(reason);
    pendingAck = null;
  };

  return new WritableStream<Uint8Array>({
    write(chunk) {
      return new Promise<void>((resolve, reject) => {
        const timeout = setTimeout(() => {
          pendingAck = null;
          reject(
            new GosdSaveFailedError(
              `the service worker stopped acknowledging chunks of "${filename}" (no ack within ${ackTimeoutMs}ms); the browser may have terminated the worker — retry, or use a different save tier`,
            ),
          );
        }, ackTimeoutMs);
        pendingAck = {
          resolve: () => {
            clearTimeout(timeout);
            resolve();
          },
          reject: (err) => {
            clearTimeout(timeout);
            reject(err);
          },
        };
        port.postMessage({ type: "chunk", chunk }, [
          chunk.buffer as unknown as Transferable,
        ]);
      });
    },
    close() {
      settlePending(new GosdSaveFailedError("stream closed mid-write"));
      port.postMessage({ type: "end" });
    },
    abort(reason) {
      settlePending(reason);
      port.postMessage({ type: "abort", reason: String(reason) });
    },
  });
}

export async function createServiceWorkerSink(
  options: CreateServiceWorkerSinkOptions,
): Promise<SaveSink> {
  const sw = getServiceWorkerContainer(
    typeof navigator === "undefined" ? undefined : navigator,
  );
  const controller = sw?.controller;
  if (!sw || !controller) {
    throw new GosdSaveFailedError(
      "the service-worker save tier needs an active, controlling service worker; register the worker this package ships at gosd/downloads/service-worker.js first — see the package README's SW hosting section",
    );
  }

  const id = randomId();
  const channel = new MessageChannel();

  const ready = new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      channel.port1.onmessage = null;
      reject(
        new GosdSaveFailedError(
          `the service worker never acknowledged download "${options.filename}" within ${READY_TIMEOUT_MS}ms; confirm gosd/downloads/service-worker.js is registered and active at scope ${options.serviceWorker.scope}`,
        ),
      );
    }, READY_TIMEOUT_MS);

    channel.port1.onmessage = (event: MessageEvent) => {
      const data = event.data as
        { type?: string; protocol?: number } | undefined;
      if (data?.type !== "ready") return;
      clearTimeout(timeout);
      if (data.protocol !== PROTOCOL) {
        reject(
          new GosdSaveFailedError(
            `the registered service worker speaks protocol ${String(data.protocol)}, but this package speaks protocol ${PROTOCOL}; update the hosted gosd-download-sw.js to the version shipped alongside this package`,
          ),
        );
        return;
      }
      resolve();
    };
  });

  controller.postMessage(
    {
      gosd: 1,
      type: "begin",
      id,
      filename: options.filename,
      size: options.size,
    },
    [channel.port2],
  );
  await ready;

  const keepalive = setInterval(() => {
    channel.port1.postMessage({ type: "ping" });
  }, PING_INTERVAL_MS);

  let downloadTriggered = false;
  function triggerDownload(): void {
    if (downloadTriggered) return;
    downloadTriggered = true;
    const url = `${options.serviceWorker.scope}gosd/${id}/${encodeURIComponent(options.filename)}`;
    const iframe = document.createElement("iframe");
    iframe.style.display = "none";
    iframe.src = url;
    document.body.appendChild(iframe);
    setTimeout(() => iframe.remove(), IFRAME_CLEANUP_MS);
  }

  let writable: WritableStream<Uint8Array>;
  if (canTransferReadableStream()) {
    const passthrough = new TransformStream<Uint8Array, Uint8Array>();
    channel.port1.postMessage(
      { type: "stream", stream: passthrough.readable },
      [passthrough.readable as unknown as Transferable],
    );
    writable = passthrough.writable;
  } else {
    writable = createPumpWritable(channel.port1, options.filename);
  }
  triggerDownload();

  return {
    kind: "service-worker",
    writable,
    async commit() {
      clearInterval(keepalive);
      channel.port1.close();
    },
    async abort(reason) {
      clearInterval(keepalive);
      try {
        await writable.abort(reason);
      } catch {
        // Already aborted via pipeTo's automatic propagation (the writable
        // errored before this explicit call ran); nothing more to do.
      }
      channel.port1.close();
    },
  };
}
