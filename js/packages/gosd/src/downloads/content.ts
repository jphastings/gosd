// Turns the caller's `{ path: content }` map into exact-size, placeholder-
// shaped byte buffers ready to splice into the image — the browser-side
// counterpart of `gosd build --placeholder`'s own rendering
// (internal/inject.Render), which always ends a placeholder's bytes on a
// `\n`. A caller may fill any subset of a manifest's placeholders; the rest
// are left untouched (read as absent, per the placeholder contract) but are
// still hash-verified during the substitution pass in substitute.ts.

import { ENV_REGION_KEY, renderEnvBody } from "./env.js";
import { GosdContentTooLargeError, GosdUnknownPlaceholderError } from "./errors.js";
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
    if (path === ENV_REGION_KEY) {
      throw new GosdUnknownPlaceholderError(
        `withPlaceholders: "${ENV_REGION_KEY}" is not a placeholder path; pass app settings as the separate \`env\` option instead, e.g. withPlaceholders(url, files, { env: { API_TOKEN: "..." } })`,
      );
    }
    const placeholder = byPath.get(path);
    if (!placeholder) {
      const available =
        manifest.placeholders.map((p) => p.path).join(", ") || "(this image has no placeholders)";
      throw new GosdUnknownPlaceholderError(
        `withPlaceholders: "${path}" is not a placeholder in this image's manifest; available placeholders: ${available}`,
      );
    }

    const body = typeof content === "string" ? new TextEncoder().encode(content) : content;
    if (body.length > placeholder.size) {
      throw new GosdContentTooLargeError(
        `withPlaceholders: content for "${path}" is ${body.length} bytes, which does not fit in its ${placeholder.size}-byte reserved placeholder; shorten the content, or reserve a larger placeholder with --placeholder at build time`,
      );
    }

    padded.set(path, padTo(body, placeholder.size));
  }

  return padded;
}

/** Pads every region a caller is filling in — placeholder files and, when
 * `env` is given, the reserved [env] region — into the one map the
 * substitution engine consumes. Both download entry points build their
 * `padded` this way, so a resumed download pads exactly as the interrupted
 * one did. */
export function padAll(
  files: Record<string, string | Uint8Array>,
  env: Record<string, string> | undefined,
  manifest: Manifest,
): Map<string, Uint8Array> {
  const padded = padContents(files, manifest);
  if (env) {
    padded.set(ENV_REGION_KEY, padEnv(env, manifest));
  }
  return padded;
}

/** Renders `env` as a TOML [env] body and pads it to the image's reserved
 * region, ready to splice like any placeholder's content. Throws when the
 * image reserved no region (nothing to write into), and when the rendered
 * settings don't fit — both at the call, before a byte is downloaded. */
export function padEnv(env: Record<string, string>, manifest: Manifest): Uint8Array {
  if (!manifest.env) {
    throw new GosdUnknownPlaceholderError(
      "withPlaceholders: this image reserved no [env] region, so its app settings can't be injected; rebuild it with `gosd build --env-placeholder <size>`",
    );
  }

  const body = new TextEncoder().encode(renderEnvBody(env));
  if (body.length > manifest.env.size) {
    throw new GosdContentTooLargeError(
      `withPlaceholders: the rendered [env] settings are ${body.length} bytes, which does not fit in this image's ${manifest.env.size}-byte reserved region; shorten them, or reserve a larger region with --env-placeholder at build time`,
    );
  }
  return padTo(body, manifest.env.size);
}

/** Pads `body` to exactly `size` with trailing `0x0A` newlines — never NUL
 * bytes, which would corrupt the text formats these regions carry. */
function padTo(body: Uint8Array, size: number): Uint8Array {
  const out = new Uint8Array(size);
  out.set(body);
  out.fill(0x0a, body.length);
  return out;
}
