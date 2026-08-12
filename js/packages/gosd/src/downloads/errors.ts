// Typed errors withPlaceholders and the functions it's built from throw.
// Every message follows the repo's convention: what failed, why, what to
// try — never a bare wrapped chain. Callers can narrow either by `instanceof`
// (one class per code) or by switching on the `code` string, which is what
// survives a `postMessage`/serialization boundary.

export type GosdErrorCode =
  | "manifest-fetch"
  | "manifest-invalid"
  | "manifest-hash-mismatch"
  | "unknown-placeholder"
  | "invalid-env"
  | "content-too-large"
  | "image-fetch"
  | "image-precondition"
  | "placeholder-not-pristine"
  | "image-hash-mismatch"
  | "image-size"
  | "save-failed"
  | "cancelled";

export class GosdError extends Error {
  readonly code: GosdErrorCode;

  constructor(code: GosdErrorCode, message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.code = code;
    this.name = "GosdError";
  }
}

/** The `<image>.inject.json` manifest could not be fetched. */
export class GosdManifestFetchError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("manifest-fetch", message, options);
    this.name = "GosdManifestFetchError";
  }
}

/** The manifest was fetched but fails structural validation. */
export class GosdManifestInvalidError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("manifest-invalid", message, options);
    this.name = "GosdManifestInvalidError";
  }
}

/** The manifest's bytes don't match a caller-pinned `manifestSha256`. */
export class GosdManifestHashMismatchError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("manifest-hash-mismatch", message, options);
    this.name = "GosdManifestHashMismatchError";
  }
}

/** `padContents` was given a key that names no placeholder in the manifest. */
export class GosdUnknownPlaceholderError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("unknown-placeholder", message, options);
    this.name = "GosdUnknownPlaceholderError";
  }
}

/** An `env` entry can't be rendered into the reserved [env] region. */
export class GosdInvalidEnvError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("invalid-env", message, options);
    this.name = "GosdInvalidEnvError";
  }
}

/** A placeholder's replacement content doesn't fit its reserved size. */
export class GosdContentTooLargeError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("content-too-large", message, options);
    this.name = "GosdContentTooLargeError";
  }
}

/** The image itself could not be fetched. */
export class GosdImageFetchError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("image-fetch", message, options);
    this.name = "GosdImageFetchError";
  }
}

/** The image response's ETag or Content-Length disagrees with the manifest,
 * before a single byte of the body has been read. */
export class GosdImagePreconditionError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("image-precondition", message, options);
    this.name = "GosdImagePreconditionError";
  }
}

/** A placeholder's current bytes don't hash to the manifest's recorded
 * SHA-256 — the image is tampered with, or already patched. */
export class GosdPlaceholderNotPristineError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("placeholder-not-pristine", message, options);
    this.name = "GosdPlaceholderNotPristineError";
  }
}

/** The whole image's streamed SHA-256 doesn't match `image.sha256`. */
export class GosdImageHashMismatchError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("image-hash-mismatch", message, options);
    this.name = "GosdImageHashMismatchError";
  }
}

/** The stream ended short of, or ran past, `image.size`. */
export class GosdImageSizeError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("image-size", message, options);
    this.name = "GosdImageSizeError";
  }
}

/** A save sink failed to commit the downloaded, verified, patched image. */
export class GosdSaveFailedError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("save-failed", message, options);
    this.name = "GosdSaveFailedError";
  }
}

/** The user dismissed a save picker before anything was fetched. */
export class GosdCancelledError extends GosdError {
  constructor(message: string, options?: { cause?: unknown }) {
    super("cancelled", message, options);
    this.name = "GosdCancelledError";
  }
}
