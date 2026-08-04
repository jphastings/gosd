// Turns the caller's `{ path: content }` map into exact-size, placeholder-
// shaped byte buffers ready to splice into the image — the browser-side
// counterpart of `gosd build --placeholder`'s own rendering
// (internal/inject.Render), which always ends a placeholder's bytes on a
// `\n`. A caller may fill any subset of a manifest's placeholders; the rest
// are left untouched (read as absent, per the placeholder contract) but are
// still hash-verified during the substitution pass in substitute.ts.

import {
  GosdContentTooLargeError,
  GosdUnknownPlaceholderError,
} from "./errors.js";
import type { Manifest } from "./manifest.js";

/** UTF-8 encodes and pads each entry of `files` to its placeholder's exact
 * reserved size with trailing `0x0A` newlines — never NUL bytes, which
 * would corrupt the text formats (YAML, TOML) placeholders exist to carry.
 * An exact-fit value is used byte for byte. Every key must name a
 * placeholder in `manifest` — otherwise `GosdUnknownPlaceholderError`
 * lists the ones that do — and no value may exceed its placeholder's size
 * — otherwise `GosdContentTooLargeError` names both sizes. Returns a map
 * keyed by placeholder path, containing only the placeholders `files`
 * touched. */
export function padContents(
  files: Record<string, string | Uint8Array>,
  manifest: Manifest,
): Map<string, Uint8Array> {
  const byPath = new Map(manifest.placeholders.map((p) => [p.path, p]));
  const padded = new Map<string, Uint8Array>();

  for (const [path, content] of Object.entries(files)) {
    const placeholder = byPath.get(path);
    if (!placeholder) {
      const available =
        manifest.placeholders.map((p) => p.path).join(", ") ||
        "(this image has no placeholders)";
      throw new GosdUnknownPlaceholderError(
        `withPlaceholders: "${path}" is not a placeholder in this image's manifest; available placeholders: ${available}`,
      );
    }

    const body =
      typeof content === "string" ? new TextEncoder().encode(content) : content;
    if (body.length > placeholder.size) {
      throw new GosdContentTooLargeError(
        `withPlaceholders: content for "${path}" is ${body.length} bytes, which does not fit in its ${placeholder.size}-byte reserved placeholder; shorten the content, or reserve a larger placeholder with --placeholder at build time`,
      );
    }

    const out = new Uint8Array(placeholder.size);
    out.set(body);
    out.fill(0x0a, body.length);
    padded.set(path, out);
  }

  return padded;
}
