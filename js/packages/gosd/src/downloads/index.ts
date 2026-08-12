// Public entry point: `import { withPlaceholders } from "@jphastings/gosd/downloads"`.
// One call fetches a gosd-built image, verifies it against its
// `<image>.inject.json` manifest, splices your content into its
// placeholder byte ranges as it streams, and saves it — auto-selecting the
// best available browser save tier (see the package README's tier table).
// Every lower-level piece this composes is also exported, for callers
// assembling their own pipeline or driving the (DOM-free) core from Node.

import { GosdCancelledError, GosdSaveFailedError } from "./errors.js";
import { deriveManifestURL, fetchManifest, type Manifest } from "./manifest.js";
import { padAll, type ConfigOption } from "./content.js";
import { runDownload } from "./run.js";
import { createFsAccessSink, fsAccessAvailable } from "./sinks/fs-access.js";
import { createMemorySink } from "./sinks/memory.js";
import {
  createServiceWorkerSink,
  serviceWorkerAvailable,
  type ServiceWorkerLocation,
} from "./sinks/service-worker.js";
import type { SaveSink, SaveSinkKind, SeekableSaveSink } from "./sinks/types.js";
import type { SubstitutionProgress } from "./substitute.js";
import { createFreshDownloadCheckpoint } from "./resume.js";
import type { ResumeStore } from "./resume-store.js";

export type SaveTier = SaveSinkKind;

export interface WithPlaceholdersOptions {
  /** Overrides the derived `<image>.inject.json` URL (see
   * `deriveManifestURL`). */
  manifestURL?: string | URL;
  /** Pins the manifest's own bytes to a known hash (e.g. an index's
   * `inject_sha256`). */
  manifestSha256?: string;
  /** Skips fetching the manifest entirely — pass one already fetched
   * (e.g. to shorten the gap between a user gesture and the save picker).
   * `manifestURL`/`manifestSha256` are ignored when this is given. */
  manifest?: Manifest;
  /** Forces a specific save tier; failure throws instead of falling back.
   * Omit to auto-select (see the package README). */
  saveVia?: SaveTier;
  /** Required to use (or auto-select) the service-worker tier: the
   * same-origin location of the worker script this package ships at the
   * `@jphastings/gosd/downloads/service-worker.js` export subpath. */
  serviceWorker?: ServiceWorkerLocation;
  /** The filename offered to the save picker / browser download. Defaults
   * to the image URL's last path segment. */
  suggestedName?: string;
  /** The gosd.toml to write into the image's reserved config region (`gosd
   * build --config-placeholder`): either a complete replacement, or — better
   * — a function handed the pristine file to edit, which keeps the
   * plain-language guidance gosd wrote for whoever opens the card.
   *
   * Whatever lands there behaves exactly like a config typed onto the card
   * by hand: it sets the hostname, WiFi, `[env]` settings and `[ingress.*]`
   * tunnel, the device needs no app code for any of it, and the provisioning
   * snapshot carries it across a later reflash (see docs/image-injection.md
   * in the gosd repo). Throws if the image reserved no region, or if the
   * result doesn't fit it. */
  config?: ConfigOption;
  ignoreETag?: boolean;
  /** Defaults to the global `fetch`. */
  fetch?: typeof fetch;
  signal?: AbortSignal;
  onProgress?: (progress: SubstitutionProgress) => void;
  /** Opt-in: on the fs-access tier, checkpoints enough progress to
   * IndexedDB (the file handle, the image's identity, and each patched
   * placeholder's pristine bytes as they verify) that an interrupted
   * download can later be continued with `resumeDownload` instead of
   * starting over. No effect on the memory or service-worker tiers — see
   * the package README's "Resuming" section. Off by default: resuming
   * needs IndexedDB writes this option's callers may not want. */
  resumable?: boolean;
  /** Overrides the default IndexedDB-backed resume store `resumable` uses
   * — mainly for tests. */
  resumeStore?: ResumeStore;
}

export interface WithPlaceholdersResult {
  savedVia: SaveTier;
  manifest: Manifest;
  /** The verified whole-image SHA-256 (always `manifest.image.sha256` —
   * the download wouldn't have completed otherwise). */
  sha256: string;
  filename: string;
}

function deriveFilenameFromURL(imageURL: string | URL): string {
  const basename = new URL(imageURL).pathname.split("/").pop();
  return basename && basename.length > 0 ? basename : "download.img";
}

async function createServiceWorkerSinkChecked(
  options: WithPlaceholdersOptions,
  filename: string,
  manifest: Manifest,
): Promise<SaveSink> {
  if (!options.serviceWorker) {
    throw new GosdSaveFailedError(
      "the service-worker save tier needs options.serviceWorker = { url, scope } naming the hosted worker script",
    );
  }
  return createServiceWorkerSink({
    filename,
    size: manifest.image.size,
    serviceWorker: options.serviceWorker,
  });
}

export async function withPlaceholders(
  imageURL: string | URL,
  files: Record<string, string | Uint8Array>,
  options: WithPlaceholdersOptions = {},
): Promise<WithPlaceholdersResult> {
  const filename = options.suggestedName ?? deriveFilenameFromURL(imageURL);
  const forced = options.saveVia;
  const preferFsAccess = forced === "fs-access" || (!forced && fsAccessAvailable());

  // MUST run before any other `await` in this function: showSaveFilePicker
  // needs "transient user activation", which a synchronous call chain from
  // a click handler carries across an `await` but not across a prior one
  // (see sinks/fs-access.ts). Everything else below is free to be async.
  let fsAccessSink: SeekableSaveSink | undefined;
  let demoted = false;
  if (preferFsAccess) {
    try {
      fsAccessSink = await createFsAccessSink({ suggestedName: filename });
    } catch (err) {
      if (forced || err instanceof GosdCancelledError) throw err;
      console.warn(
        `gosd: the fs-access save tier failed (${String(err)}); falling back to another tier`,
      );
      demoted = true;
    }
  }

  try {
    const manifest =
      options.manifest ??
      (await fetchManifest(options.manifestURL ?? deriveManifestURL(imageURL), {
        fetch: options.fetch,
        manifestSha256: options.manifestSha256,
        signal: options.signal,
      }));
    const padded = padAll(files, options.config, manifest);

    let sink: SaveSink;
    let tier: SaveTier;

    if (fsAccessSink) {
      sink = fsAccessSink;
      tier = "fs-access";
    } else if (forced === "service-worker") {
      sink = await createServiceWorkerSinkChecked(options, filename, manifest);
      tier = "service-worker";
    } else if (forced === "memory") {
      sink = createMemorySink({ filename });
      tier = "memory";
    } else if (forced) {
      throw new GosdSaveFailedError(`withPlaceholders: unknown save tier "${String(forced)}"`);
    } else if (options.serviceWorker && (await serviceWorkerAvailable(options.serviceWorker))) {
      try {
        sink = await createServiceWorkerSinkChecked(options, filename, manifest);
        tier = "service-worker";
      } catch (err) {
        console.warn(
          `gosd: the service-worker save tier failed (${String(err)}); falling back to the memory tier`,
        );
        sink = createMemorySink({ filename });
        tier = "memory";
        demoted = true;
      }
    } else {
      sink = createMemorySink({ filename });
      tier = "memory";
    }

    const checkpoint =
      options.resumable && fsAccessSink
        ? createFreshDownloadCheckpoint({
            sink: fsAccessSink,
            manifest,
            imageURL: String(imageURL),
            filename,
            store: options.resumeStore,
          })
        : undefined;

    const result = await runDownload({
      manifest,
      padded,
      fetchImage: () => (options.fetch ?? fetch)(imageURL, { signal: options.signal }),
      sink,
      ignoreETag: options.ignoreETag,
      signal: options.signal,
      onProgress: options.onProgress,
      checkpoint,
    });

    if (demoted) {
      console.warn(
        `gosd: withPlaceholders saved via the "${tier}" tier after an earlier tier failed`,
      );
    }

    return { savedVia: tier, manifest, sha256: result.sha256, filename };
  } catch (err) {
    if (fsAccessSink) {
      await fsAccessSink.abort(err).catch(() => {});
    }
    throw err;
  }
}

export {
  GosdError,
  GosdManifestFetchError,
  GosdManifestInvalidError,
  GosdManifestHashMismatchError,
  GosdUnknownPlaceholderError,
  GosdInvalidEnvError,
  GosdContentTooLargeError,
  GosdImageFetchError,
  GosdImagePreconditionError,
  GosdPlaceholderNotPristineError,
  GosdImageHashMismatchError,
  GosdImageSizeError,
  GosdSaveFailedError,
  GosdCancelledError,
} from "./errors.js";
export type { GosdErrorCode } from "./errors.js";

export { deriveManifestURL, fetchManifest, parseManifest } from "./manifest.js";
export type {
  Manifest,
  ImageInfo,
  PlaceholderInfo,
  ConfigInfo,
  RegionInfo,
  ByteRange,
  FetchManifestOptions,
} from "./manifest.js";
export { injectableRegions } from "./manifest.js";

export { padContents, padConfig, padAll } from "./content.js";
export type { ConfigOption } from "./content.js";

export { renderEnvBody } from "./env.js";

export { createSubstitutionTransform, patchStream } from "./substitute.js";
export type { SubstitutionProgress, SubstitutionOptions } from "./substitute.js";

export { checkImageResponse } from "./preconditions.js";
export type { CheckImageResponseOptions } from "./preconditions.js";

export { runDownload } from "./run.js";
export type {
  RunDownloadOptions,
  RunDownloadResult,
  ImageResponseProvider,
  DownloadCheckpoint,
} from "./run.js";

export { Sha256 } from "./sha256.js";

export type { SaveSink, SaveSinkKind, SeekableSaveSink } from "./sinks/types.js";
export { isSeekable } from "./sinks/types.js";
export { createMemorySink } from "./sinks/memory.js";
export type { CreateMemorySinkOptions } from "./sinks/memory.js";
export { createFsAccessSink, fsAccessAvailable } from "./sinks/fs-access.js";
export type { CreateFsAccessSinkOptions } from "./sinks/fs-access.js";
export {
  createServiceWorkerSink,
  serviceWorkerAvailable,
  PROTOCOL as SERVICE_WORKER_PROTOCOL,
} from "./sinks/service-worker.js";
export type {
  CreateServiceWorkerSinkOptions,
  ServiceWorkerLocation,
} from "./sinks/service-worker.js";

export {
  createFreshDownloadCheckpoint,
  listResumableDownloads,
  discardResumableDownload,
  resumeDownload,
  resumeStoreAvailable,
  createIndexedDbResumeStore,
} from "./resume.js";
export type {
  CreateFreshDownloadCheckpointOptions,
  ResumableDownloadInfo,
  ListResumableDownloadsOptions,
  DiscardResumableDownloadOptions,
  ResumeDownloadOptions,
  ResumeDownloadResult,
  ResumeRecord,
  ResumeStore,
} from "./resume.js";
