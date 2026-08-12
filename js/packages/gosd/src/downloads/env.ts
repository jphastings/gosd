// Renders a TOML `[env]` table body. The package doesn't need this itself —
// `options.config` carries whole gosd.toml text — but assembling one by hand
// is fiddly in exactly the ways that bite silently on a device (escaping, and
// the `GOSD_*` names gosd-init logs-and-ignores), so it's exported for
// callers building an `[env]` section inside their own config edit.
//
// Only the body — `KEY = "value"` lines — is produced; the caller places it
// under an `[env]` header. Keys are sorted so the same settings always
// produce the same bytes.

import { GosdInvalidEnvError } from "./errors.js";

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
