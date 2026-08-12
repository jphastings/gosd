// Turns the caller's `{ path: content }` map into exact-size, placeholder-
// shaped byte buffers ready to splice into the image — the browser-side
// counterpart of `gosd build --placeholder`'s own rendering
// (internal/inject.Render), which always ends a placeholder's bytes on a
// `\n`. A caller may fill any subset of a manifest's placeholders; the rest
// are left untouched (read as absent, per the placeholder contract) but are
// still hash-verified during the substitution pass in substitute.ts.

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
    if (manifest.config && path === manifest.config.path) {
      throw new GosdUnknownPlaceholderError(
        `withPlaceholders: "${path}" is this image's reserved config file, not a placeholder; pass it as the separate \`config\` option instead, e.g. withPlaceholders(url, files, { config: (current) => current.replace(...) })`,
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
  config: ConfigOption | undefined,
  manifest: Manifest,
): Map<string, Uint8Array> {
  const padded = padContents(files, manifest);
  if (config !== undefined) {
    const region = manifest.config;
    if (!region) {
      throw new GosdUnknownPlaceholderError(
        "withPlaceholders: this image reserved no space for its gosd.toml, so its configuration can't be injected; rebuild it with `gosd build --config-placeholder`",
      );
    }
    padded.set(region.path, padConfig(config, manifest));
  }
  return padded;
}

/** A replacement gosd.toml, or a function handed the pristine one to edit —
 * editing keeps the plain-language guidance gosd wrote for whoever opens the
 * card, which a generated replacement would drop. */
export type ConfigOption = string | ((pristine: string) => string);

/** Resolves `config` against the manifest's published pristine text and pads
 * the result to the reserved size. Throws when the image reserved no config
 * region, and when the result doesn't fit — both before a byte is
 * downloaded. */
export function padConfig(config: ConfigOption, manifest: Manifest): Uint8Array {
  const region = manifest.config;
  if (!region) {
    throw new GosdUnknownPlaceholderError(
      "withPlaceholders: this image reserved no space for its gosd.toml, so its configuration can't be injected; rebuild it with `gosd build --config-placeholder`",
    );
  }

  const text = typeof config === "function" ? config(region.pristine) : config;
  const body = new TextEncoder().encode(text);
  if (body.length > region.size) {
    throw new GosdContentTooLargeError(
      `withPlaceholders: the gosd.toml to inject is ${body.length} bytes, which does not fit in this image's ${region.size}-byte reserved region; shorten it, or reserve more with --config-placeholder at build time`,
    );
  }
  return padTo(body, region.size);
}

/** Pads `body` to exactly `size` with trailing `0x0A` newlines — never NUL
 * bytes, which would corrupt the text formats these regions carry. */
function padTo(body: Uint8Array, size: number): Uint8Array {
  const out = new Uint8Array(size);
  out.set(body);
  out.fill(0x0a, body.length);
  return out;
}
