import { describe, expect, it } from "vitest";
import * as errors from "./errors.js";

const classesByCode: Record<errors.GosdErrorCode, new (message: string) => errors.GosdError> = {
  "manifest-fetch": errors.GosdManifestFetchError,
  "manifest-invalid": errors.GosdManifestInvalidError,
  "manifest-hash-mismatch": errors.GosdManifestHashMismatchError,
  "unknown-placeholder": errors.GosdUnknownPlaceholderError,
  "content-too-large": errors.GosdContentTooLargeError,
  "image-fetch": errors.GosdImageFetchError,
  "image-precondition": errors.GosdImagePreconditionError,
  "placeholder-not-pristine": errors.GosdPlaceholderNotPristineError,
  "image-hash-mismatch": errors.GosdImageHashMismatchError,
  "image-size": errors.GosdImageSizeError,
  "save-failed": errors.GosdSaveFailedError,
  cancelled: errors.GosdCancelledError,
};

describe("typed errors", () => {
  it.each(Object.entries(classesByCode))(
    "%s sets .code and is an instanceof GosdError and Error",
    (code, ErrorClass) => {
      const err = new ErrorClass("boom");
      expect(err.code).toBe(code);
      expect(err).toBeInstanceOf(errors.GosdError);
      expect(err).toBeInstanceOf(Error);
      expect(err.message).toBe("boom");
    },
  );

  it("preserves a `cause`", () => {
    const cause = new Error("root cause");
    const err = new errors.GosdImageFetchError("wrapped", { cause });
    expect(err.cause).toBe(cause);
  });
});
