---
# gosd-hcyn
title: 'js/: gosd npm package — gosd/downloads placeholder-substituting downloader'
status: completed
type: feature
created_at: 2026-08-04T08:57:17Z
updated_at: 2026-08-04T08:57:17Z
---

Add a JavaScript/TypeScript area (js/, npm workspaces) and an npm package named gosd with subpath export gosd/downloads: withPlaceholders(imageURL, files) downloads an image, verifies everything against its .inject.json manifest (untrusted-host model), splices padded file contents into the manifest's byte ranges on the fly, and saves via tiered browser mechanisms. Zero runtime dependencies.

## Locked decisions (JP, 2026-08-04)

- **Package name `gosd`** (bare — verified unregistered on npm; the @gosd scope is not claimable). Subpath export `gosd/downloads`. Fallback if sniped before publish: `@jphastings/gosd` — one-line change, subpath unchanged.
- **Save tiers**: (1) File System Access API streaming (Chromium; picker called synchronously in the user gesture, before any await); (2) opt-in service-worker streaming when the integrator hosts our shipped worker script and passes `options.serviceWorker`; (3) memory buffer + Blob anchor as last resort — this tier completes ALL verification before anything is saved. `saveVia` forces a tier; auto-demotions warn and surface in the result.
- **ETag policy: fail-fast only.** An ETag (stripped of W/ and quotes) matching /^[0-9a-f]{64}$/i that differs from manifest image.sha256 aborts before writing; a match NEVER skips the full streamed hash (the download host is untrusted; only the manifest is trusted). Content-Length ≠ image.size also fails fast (skipped when content-encoding is present). `ignoreETag` escape hatch.
- **Zero runtime dependencies**: vendored streaming SHA-256 class (~150 lines) in src/downloads/sha256.ts, pinned by NIST CAVP vectors + random cross-checks against crypto.subtle.digest + chunked==one-shot tests. WebCrypto alone can't hash incrementally. No secrets hashed, so timing is irrelevant.
- **Resuming is OUT of scope** — follow-up bean. Sink interface stays sequential-only; hasher exposes clone() to keep in-session checkpointing possible.
- **Tooling** (aligned with atbackup/web): npm workspaces at js/ root, TypeScript ~6 strict (module/moduleResolution nodenext, explicit ./x.js import specifiers), plain tsc build (two programs: DOM lib and WebWorker SW — no bundler), Vitest 4 (unit + integration projects), Prettier. The SW file src/sw/gosd-download-sw.ts has ZERO imports/exports (classic script; module SWs not universal) — a test asserts the emitted file has none.
- **Semantics mirror Go internal/inject**: manifest URL = image URL with LAST path extension swapped for .inject.json (filepath.Ext semantics); padding = trailing 0x0A to exact placeholder size; subset of placeholders fine (untouched ones verified pristine and left alone); content = concatenation of a placeholder's ranges in order (fragmented allocation legal).
- **Cross-implementation fixture**: internal/cmd/injectfixture (Go, in-module, precedent imgextract/qemuboot) generates a real small .img + .inject.json via internal/image.Write + internal/inject.WriteManifest; the vitest integration project consumes it through the Node-compatible core with an injected fetch. `npm test` stays Go-free; `npm run test:integration` runs the generator.
- Full plan: ~/.claude/plans/can-you-please-create-harmonic-bunny.md (this bean carries the decisions; the plan carries the algorithms).

## Todos

- [x] Scaffold js/ workspace (root package.json + lockfile, tsconfig.base, prettier config) + .gitignore entries
- [x] Core modules test-first: errors, sha256 (NIST vectors + WebCrypto cross-check), manifest/deriveManifestURL, content padding, substitute (chunk-boundary matrix vs naive reference), preconditions (ETag/Content-Length), run orchestration
- [x] Sinks: memory, fs-access (gesture-safe ordering + call-order test), service-worker client + src/sw worker script + protocol tests
- [x] internal/cmd/injectfixture + vitest integration test + npm scripts
- [x] CI: js job in .github/workflows/ci.yml (node 20/24 matrix, SHA-pinned actions)
- [x] Docs: packages/gosd/README.md, docs/image-injection.md pointer, root README bullet, CLAUDE.md carve-out + quality-gates line
- [x] Quality gates: full Go set AND cd js && npm ci && format:check && typecheck && build && test && test:integration

## Follow-ups (created)

- gosd-bs9s — download resuming (FS-Access tier: Range/If-Range, persisted handle, IndexedDB state, partial-file re-verification)
- gosd-xe3r — npm publishing (JP registers the gosd name; manual publish first, automation later)

## Summary of Changes

- New `js/` npm-workspaces area (root package.json/lockfile, shared strict tsconfig.base, Prettier) and the `gosd` package at `js/packages/gosd` with subpath export `gosd/downloads`. Zero runtime dependencies; plain-tsc build (separate DOM and WebWorker programs); a third `tsconfig.core.json --noEmit` program compiles the core against `lib: ES2022` only, mechanically enforcing that everything below the sink layer runs on Node 20.
- **Core**: typed `GosdError` subclasses with literal codes; `deriveManifestURL` (Go `filepath.Ext` semantics); `parseManifest`/`fetchManifest` (structural validation naming JSON paths, Σranges==size, in-bounds, global overlap check, optional sha256 pin — normalized + shape-checked); `padContents` (trailing-`0x0A` padding to exact size — review caught the subagent zero-filling, which would have put NUL bytes into YAML); vendored streaming `Sha256` (NIST CAVP vectors, WebCrypto cross-checks, chunked==one-shot, clone()); `createSubstitutionTransform` (globally sorted segment plan, original-bytes hashing, mid-stream per-placeholder pristine verification the instant each placeholder's last byte passes, copy-on-write only for chunks a replacement touches, short/overlong-stream errors, final whole-image digest at flush); `checkImageResponse` (fail-fast ETag/Content-Length, never skipping the streamed hash); `runDownload` (sink aborted on every failure path, commit only after a full successful pipe).
- **Save tiers**: fs-access (picker synchronously within the user gesture — before any await; swap-file semantics mean failures never leave partial bytes), opt-in service-worker streaming (shipped import-free classic worker at the `gosd/downloads/service-worker.js` subpath; MessageChannel handshake with PROTOCOL literal match test; transferable-stream path plus a pump fallback that got push-with-ack backpressure in review — one chunk in flight, ack timeout doubling as the dead-worker watchdog), and memory+Blob (all verification completes before anything is saved).
- **Cross-implementation proof**: `internal/cmd/injectfixture` (Go, module code) builds a real 24MiB image + manifest via internal/image + internal/inject; the vitest integration project patches it through the core with an injected fetch and byte-compares everything (patched ranges exact, all other bytes identical, untouched placeholder pristine, corruption cases typed).
- **CI**: `js` job (node 20/24 matrix, SHA-pinned setup-node v7.0.0 — pin verified against the tag via git ls-remote — plus setup-go for the fixture) running format:check/typecheck/build/test/test:integration.
- Docs: js/packages/gosd/README.md (quickstart, threat model, tier table, SW hosting, error codes), docs/image-injection.md pointer, root README bullet, CLAUDE.md carve-out + js quality-gates line, .gitignore entries.
- 111 unit + 4 integration tests; all Go gates unaffected and green.
