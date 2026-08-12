---
# gosd-ypyz
title: 'npm package: fill the reserved [env] region from @jphastings/gosd/downloads'
status: completed
type: feature
priority: normal
created_at: 2026-08-12T13:04:54Z
updated_at: 2026-08-12T14:04:02Z
---

`gosd build --env-placeholder` (bean gosd-dwub) publishes the reserved `[env]`
region as a top-level `env` key in `<image>.inject.json`. The npm package keys
everything off `placeholders[].path`, so it currently ignores that key: a
browser client can inject files but not settings, and has to hand-roll the
splice.

Deliberately NOT modelled as a pseudo-entry in `placeholders[]` (which would
have worked in today's client unchanged): `path` is documented as a real
FAT-root path and `size` as that file's whole size, and a client that acted on
either belief would corrupt gosd.toml.

## Shape

- `withPlaceholders(imageURL, files, { env: { KEY: "value" } })` — `env` sits
  alongside `files` rather than inside it, since it isn't a path.
- Render a TOML `[env]` body from the object: `KEY = "value"` per entry, with
  TOML string escaping (`"` `\` and control characters) and a refusal for keys
  that aren't `[A-Za-z_][A-Za-z0-9_]*` or that start with `GOSD_` (the CLI
  refuses both; a client that doesn't would produce a card whose values are
  silently dropped at boot). Zero runtime dependencies, so this is ~30 hand-
  written lines, not a TOML library.
- Pad to `env.size` with newlines, refuse up front when it doesn't fit, and
  verify the pristine region against `env.sha256` exactly like a placeholder's
  before writing — the resumable path included.

## Todos

- [x] Extend `internal/cmd/injectfixture` to build a fixture with a reserved
      `[env]` region, so the cross-implementation integration test covers it
- [x] Implement the renderer + splice, with the same pristine-verification and
      failure semantics as `files`
- [x] Unit tests for escaping/refusals; integration test asserting the patched
      fixture's gosd.toml parses to the injected settings
- [x] README: the `env` option, and why an injected setting behaves like a
      hand-edit on the card (reflash survival)
- [x] `js/` gates: `pnpm install --frozen-lockfile && pnpm run format:check &&
      pnpm run lint && pnpm run typecheck && pnpm run build && pnpm test &&
      pnpm run test:integration`


## Summary of Changes

- **`options.env` on `withPlaceholders`** (and on `resumeDownload`, which must
  be given the same settings the interrupted attempt used). It sits alongside
  `files` rather than inside it, because the region is a span of gosd.toml,
  not a file: `files` keys are paths, and `"[env]"` passed as one is refused
  with a message naming the option to use instead.
- **`env.ts`** renders the TOML body: sorted `KEY = "value"` lines, basic-string
  escaping (`\` `"` `\b` `\t` `\n` `\f` `\r`, other C0/DEL as `\uXXXX`), and
  refusals for a key that isn't `[A-Za-z_][A-Za-z0-9_]*`, a `GOSD_*` key (the
  device logs-and-ignores those, so injecting one would silently do nothing),
  and a non-string value. New `GosdInvalidEnvError` / `invalid-env` code.
- **One region abstraction rather than a second code path.** `manifest.ts` gains
  `injectableRegions()`, returning placeholders and the `[env]` region as
  `{key, label, size, sha256, ranges}`. `substitute.ts`, `resume.ts`'s
  clamp/reconstruct passes, and the overlap check all iterate that, so the env
  region inherits mid-stream pristine verification, copy-on-write substitution,
  progress, and resumability with no duplicated logic. Errors read "the
  reserved [env] region is not pristine" via the region's label.
- **`injectfixture` now builds the fixture's gosd.toml with a real reserved
  region** (`gosdtoml.RenderWithReservedEnv`) instead of a token file, so the
  cross-implementation test proves the client against bytes gosd actually
  produces. Three integration cases: the pristine region really is the rendered
  `[env]` body, a splice lands exactly in its ranges with every other byte of
  the image identical, and a `GOSD_*` key is refused before any download.
- **Manifest parsing** accepts `env` as optional — a manifest without it (every
  one written before this feature) parses unchanged — and validates it exactly
  as a placeholder: ranges non-empty, summing to `size`, inside the image, and
  non-overlapping with every placeholder's.
- **README** gains an "App settings (`options.env`)" section explaining why an
  injected setting behaves like a hand-edit on the card (reflash survival,
  automatic crash-report redaction, no app code), plus the new error row.
- **Not done here:** no version bump or publish. Per `js/PUBLISHING.md` a
  release is its own PR bumping `js/packages/gosd/package.json`, then an
  `npm/gosd/vX.Y.Z` tag — JP's call.
