// Renders the TOML `[env]` body that goes into an image's reserved [env]
// region (`gosd build --env-placeholder`; see docs/image-injection.md in the
// gosd repo). What lands there is an ordinary gosd.toml setting, so this is
// the one place in the package that has to speak the device's config format
// rather than treating bytes as opaque.
//
// Only the body — `KEY = "value"` lines — is written: gosd already wrote the
// `[env]` header above the region, and a section header of our own would
// capture every setting gosd wrote below it. Keys are sorted so the same
// settings always produce the same bytes.

import { GosdInvalidEnvError } from "./errors.js";

/** The key the reserved [env] region uses in the internal padded-content and
 * captured-pristine maps. Square brackets can't appear in a placeholder path
 * (gosd restricts those to `[A-Za-z0-9._-]`), so it can never collide with
 * one. */
export const ENV_REGION_KEY = "[env]";

/** Mirrors gosd's own envKeyPattern (cmd/gosd): the shape a settings name
 * must have to reach an app. */
const ENV_KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** TOML basic-string escapes, per the spec's table. Anything else in the C0
 * range (plus DEL) becomes a \uXXXX escape. */
const SIMPLE_ESCAPES: Record<string, string> = {
  "\\": "\\\\",
  '"': '\\"',
  "\b": "\\b",
  "\t": "\\t",
  "\n": "\\n",
  "\f": "\\f",
  "\r": "\\r",
};

function escapeTomlString(value: string): string {
  let out = "";
  for (const ch of value) {
    const simple = SIMPLE_ESCAPES[ch];
    if (simple !== undefined) {
      out += simple;
      continue;
    }
    const code = ch.codePointAt(0) ?? 0;
    if (code <= 0x1f || code === 0x7f) {
      out += `\\u${code.toString(16).padStart(4, "0")}`;
      continue;
    }
    out += ch;
  }
  return out;
}

/** Renders `env` as a TOML `[env]` table body: one sorted `KEY = "value"`
 * line each, values escaped as TOML basic strings.
 *
 * Refuses what the device would refuse anyway, but at build-your-download
 * time where the message can still reach a developer: a key that isn't
 * `[A-Za-z_][A-Za-z0-9_]*`, and a `GOSD_*` key (gosd-init keeps that
 * namespace for itself and logs-and-ignores any card that claims it, so
 * injecting one would silently do nothing). */
export function renderEnvBody(env: Record<string, string>): string {
  let out = "";
  for (const key of Object.keys(env).sort()) {
    if (!ENV_KEY_PATTERN.test(key)) {
      throw new GosdInvalidEnvError(
        `withPlaceholders: env key ${JSON.stringify(key)} is not a valid environment variable name; use only letters, digits and underscores, and don't start with a digit`,
      );
    }
    if (key.startsWith("GOSD_")) {
      throw new GosdInvalidEnvError(
        `withPlaceholders: env key ${key} is in the GOSD_* namespace gosd-init reserves for itself; the device would ignore it, so rename it`,
      );
    }
    const value = env[key];
    if (typeof value !== "string") {
      throw new GosdInvalidEnvError(
        `withPlaceholders: env value for ${key} is ${typeof value}, not a string; every environment variable reaches the app as a string, so convert it yourself and keep the formatting you intended`,
      );
    }
    out += `${key} = "${escapeTomlString(value)}"\n`;
  }
  return out;
}
