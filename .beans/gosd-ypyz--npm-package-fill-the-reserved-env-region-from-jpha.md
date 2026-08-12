---
# gosd-ypyz
title: 'npm package: fill the reserved [env] region from @jphastings/gosd/downloads'
status: todo
type: feature
created_at: 2026-08-12T13:04:54Z
updated_at: 2026-08-12T13:04:54Z
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

- [ ] Extend `internal/cmd/injectfixture` to build a fixture with a reserved
      `[env]` region, so the cross-implementation integration test covers it
- [ ] Implement the renderer + splice, with the same pristine-verification and
      failure semantics as `files`
- [ ] Unit tests for escaping/refusals; integration test asserting the patched
      fixture's gosd.toml parses to the injected settings
- [ ] README: the `env` option, and why an injected setting behaves like a
      hand-edit on the card (reflash survival)
- [ ] `js/` gates: `pnpm install --frozen-lockfile && pnpm run format:check &&
      pnpm run lint && pnpm run typecheck && pnpm run build && pnpm test &&
      pnpm run test:integration`
