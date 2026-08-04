import { describe, expect, it } from "vitest";
import { checkImageResponse } from "./preconditions.js";
import { GosdImagePreconditionError } from "./errors.js";
import type { Manifest } from "./manifest.js";

function manifest(size = 1000, sha256 = "a".repeat(64)): Manifest {
  return {
    gosd_inject: 1,
    board: "test",
    image: { filename: "app.img", size, sha256 },
    placeholders: [],
  };
}

function response(headers: Record<string, string>): Response {
  return new Response(null, { headers });
}

describe("checkImageResponse: ETag", () => {
  it("passes when there's no ETag header at all", () => {
    expect(() => checkImageResponse(response({}), manifest())).not.toThrow();
  });

  it("passes when a bare 64-hex ETag matches image.sha256", () => {
    const sha = "b".repeat(64);
    expect(() =>
      checkImageResponse(response({ etag: sha }), manifest(1000, sha)),
    ).not.toThrow();
  });

  it("passes for a quoted ETag after stripping quotes", () => {
    const sha = "b".repeat(64);
    expect(() =>
      checkImageResponse(response({ etag: `"${sha}"` }), manifest(1000, sha)),
    ).not.toThrow();
  });

  it("passes for a weak (W/) ETag after stripping the weak prefix", () => {
    const sha = "b".repeat(64);
    expect(() =>
      checkImageResponse(response({ etag: `W/"${sha}"` }), manifest(1000, sha)),
    ).not.toThrow();
  });

  it("throws GosdImagePreconditionError when a 64-hex ETag disagrees with image.sha256", () => {
    expect(() =>
      checkImageResponse(
        response({ etag: "b".repeat(64) }),
        manifest(1000, "c".repeat(64)),
      ),
    ).toThrow(GosdImagePreconditionError);
  });

  it("ignores an ETag that isn't a bare 64-hex string (opaque validator)", () => {
    expect(() =>
      checkImageResponse(response({ etag: '"abc123-opaque"' }), manifest()),
    ).not.toThrow();
  });

  it("skips the ETag check entirely when ignoreETag is set", () => {
    expect(() =>
      checkImageResponse(
        response({ etag: "b".repeat(64) }),
        manifest(1000, "c".repeat(64)),
        { ignoreETag: true },
      ),
    ).not.toThrow();
  });
});

describe("checkImageResponse: Content-Length", () => {
  it("passes when Content-Length matches image.size", () => {
    expect(() =>
      checkImageResponse(
        response({ "content-length": "1000" }),
        manifest(1000),
      ),
    ).not.toThrow();
  });

  it("throws GosdImagePreconditionError when Content-Length disagrees with image.size", () => {
    expect(() =>
      checkImageResponse(response({ "content-length": "999" }), manifest(1000)),
    ).toThrow(GosdImagePreconditionError);
  });

  it("skips the Content-Length check when content-encoding is present", () => {
    expect(() =>
      checkImageResponse(
        response({ "content-length": "1", "content-encoding": "gzip" }),
        manifest(1000),
      ),
    ).not.toThrow();
  });

  it("passes when there's no Content-Length header at all", () => {
    expect(() =>
      checkImageResponse(response({}), manifest(1000)),
    ).not.toThrow();
  });
});

describe("checkImageResponse: combined matrix", () => {
  it("a matching ETag never substitutes for the Content-Length check", () => {
    const sha = "b".repeat(64);
    expect(() =>
      checkImageResponse(
        response({ etag: sha, "content-length": "1" }),
        manifest(1000, sha),
      ),
    ).toThrow(GosdImagePreconditionError);
  });
});
