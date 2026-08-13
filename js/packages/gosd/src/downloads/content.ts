// Turns the caller's `{ path: content }` map into exact-size, placeholder-
// shaped byte buffers ready to splice into the image — the browser-side
// counterpart of `gosd build --placeholder`'s own rendering
// (internal/inject.Render), which always ends a placeholder's bytes on a
// `\n` — and does the same for `{ setting: value }` against the image's
// config tree, whose value files gosd itself pads with trailing newlines
// (internal/configtree). A caller may fill any subset of either; the rest
// are left untouched (read as absent or unset) but are still hash-verified
// during the substitution pass in substitute.ts.

import {
  GosdContentTooLargeError,
  GosdInvalidEnvError,
  GosdUnknownConfigError,
  GosdUnknownPlaceholderError,
} from "./errors.js";
import { CONFIG_DIR, configRegionKey, type Manifest } from "./manifest.js";

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
        manifest.placeholders.map((p) => p.path).join(", ") || "(this image has no placeholders)";
      const settings = manifest.config.some((c) => c.path === path)
        ? `; "${path}" is one of this image's settings, so pass it as the separate \`config\` option instead`
        : "";
      throw new GosdUnknownPlaceholderError(
        `withPlaceholders: "${path}" is not a placeholder in this image's manifest; available placeholders: ${available}${settings}`,
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

/** Pads every region a caller is filling in — placeholder files and config
 * tree settings — into the one map the substitution engine consumes. Both
 * download entry points build their `padded` this way, so a resumed
 * download pads exactly as the interrupted one did. */
export function padAll(
  files: Record<string, string | Uint8Array>,
  config: Record<string, string> | undefined,
  manifest: Manifest,
): Map<string, Uint8Array> {
  const padded = padContents(files, manifest);
  if (config) {
    for (const [key, value] of padConfig(config, manifest)) {
      padded.set(key, value);
    }
  }
  return padded;
}

/** Mirrors gosd's own env name rules (internal/configtree): the shape a
 * setting under `env/` must have to reach an app. */
const ENV_KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** UTF-8 encodes each setting and pads it to that value file's reservation
 * with trailing newlines, exactly as gosd pads the pristine file. Keys are
 * tree paths with no leading `config/` (`"wifi/ssid"`, `"env/API_TOKEN"`),
 * and the returned map is keyed the way the substitution engine wants
 * (`configRegionKey`).
 *
 * Everything is refused here, at the call, before a byte is downloaded: an
 * image with no settings at all, a path this image doesn't have (the error
 * lists the ones it does), a value too long for its reservation — which is
 * fixed at build time and can never grow afterwards — and an `env/GOSD_*`
 * or otherwise unusable environment variable name, which the device would
 * ignore. */
export function padConfig(
  config: Record<string, string>,
  manifest: Manifest,
): Map<string, Uint8Array> {
  const entries = Object.entries(config);
  if (entries.length > 0 && manifest.config.length === 0) {
    throw new GosdUnknownConfigError(
      `withPlaceholders: this image has no ${CONFIG_DIR}/ settings to fill in, so \`config\` has nothing to write into; rebuild the image with a gosd version that ships a config tree`,
    );
  }

  const byPath = new Map(manifest.config.map((c) => [c.path, c]));
  const padded = new Map<string, Uint8Array>();

  for (const [path, value] of entries) {
    // Before the lookup: gosd refuses a GOSD_* (or otherwise unusable)
    // environment variable at build time, so such a path is never in the
    // manifest, and "that name is reserved" is the message that actually
    // tells a developer what to do about it.
    checkEnvPath(path);
    const setting = byPath.get(path);
    if (!setting) {
      throw new GosdUnknownConfigError(
        `withPlaceholders: "${path}" is not a setting in this image; available settings: ${manifest.config.map((c) => c.path).join(", ")}`,
      );
    }
    if (typeof value !== "string") {
      throw new GosdInvalidEnvError(
        `withPlaceholders: the value for "${path}" is ${typeof value}, not a string; every setting reaches the device as text, so convert it yourself and keep the formatting you intended`,
      );
    }

    const body = new TextEncoder().encode(value);
    if (body.length > setting.size) {
      throw new GosdContentTooLargeError(
        `withPlaceholders: the value for "${path}" is ${body.length} bytes, which does not fit in its ${setting.size}-byte reservation; shorten it, or ship a longer file for that setting at build time (a value file reserves its own size)`,
      );
    }
    padded.set(configRegionKey(path), padTo(body, setting.size));
  }

  return padded;
}

/** Refuses a setting under `env/` the device would ignore anyway, at
 * build-your-download time where the message can still reach a developer. */
function checkEnvPath(path: string): void {
  const name = path.startsWith("env/") ? path.slice("env/".length) : undefined;
  if (name === undefined) return;

  if (name.startsWith("GOSD_")) {
    throw new GosdInvalidEnvError(
      `withPlaceholders: "${path}" is in the GOSD_* namespace gosd reserves for itself; the device ignores those names, so rename the setting`,
    );
  }
  if (!ENV_KEY_PATTERN.test(name)) {
    throw new GosdInvalidEnvError(
      `withPlaceholders: "${path}" is not a valid environment variable name; use only letters, digits and underscores, and don't start with a digit`,
    );
  }
}

/** Pads `body` to exactly `size` with trailing `0x0A` newlines — never NUL
 * bytes, which would corrupt the text formats these regions carry. */
function padTo(body: Uint8Array, size: number): Uint8Array {
  const out = new Uint8Array(size);
  out.set(body);
  out.fill(0x0a, body.length);
  return out;
}
