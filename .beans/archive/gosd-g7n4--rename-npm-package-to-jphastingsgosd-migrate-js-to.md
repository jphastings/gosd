---
# gosd-g7n4
title: Rename npm package to @jphastings/gosd; migrate js/ to pnpm + Vite+
status: completed
type: task
priority: normal
created_at: 2026-08-04T12:24:06Z
updated_at: 2026-08-04T12:59:10Z
---

npm's typosquat guard 403'd the bare name gosd (too similar to got/gopd/tsd/gts/jose/os/zod), exactly the fallback case planned in gosd-hcyn: rename to @jphastings/gosd (imports become @jphastings/gosd/downloads; the tag scheme stays npm/gosd/v* keyed on the DIRECTORY name, with the workflow reading the npm name from the manifest). At the same time, migrate the js/ workspace from npm+prettier+plain-tsc to pnpm + Vite+ (vp: tsdown lib build, vitest, oxlint, oxfmt) per JP's request. Publishing stays on the npm CLI: pnpm has no OIDC trusted-publishing support yet (pnpm/pnpm#9812).

## Locked decisions

- **npm name `@jphastings/gosd`** (bare `gosd` 403'd by npm's typosquat similarity guard 2026-08-04). Import: `@jphastings/gosd/downloads`; SW asset: `@jphastings/gosd/downloads/service-worker.js`. Directory stays `js/packages/gosd`.
- **Tags stay directory-keyed**: `npm/gosd/v<version>` (no `@`/extra slash in tags); publish-npm.yml resolves the npm NAME from the package manifest and threads it to publish/smoke.
- **pnpm** replaces npm workspaces (pnpm-workspace.yaml, pnpm-lock.yaml, `packageManager` pin); CI uses pnpm/action-setup (SHA-pinned) + setup-node pnpm cache.
- **Vite+ (`vite-plus` devDependency, vp CLI)** replaces the tsc-emit + prettier toolchain: tsdown-based library build (dist layout and exports map UNCHANGED: dist/downloads/index.js + d.ts; dist/sw/gosd-download-sw.js as a classic no-import/export script), vitest via vp test, oxlint via vp lint, oxfmt replaces prettier. TypeScript stays as a devDep for the three --noEmit enforcement programs (core-no-DOM, sw WebWorker, typecheck-with-tests).
- **Publishing stays on the npm CLI** (`npm publish --provenance --access public --tag next`): pnpm has no OIDC trusted-publishing support (pnpm/pnpm#9812), and the staged/tokenless properties from gosd-hcyn are non-negotiable.
- Fallback clause: if vite-plus 0.2.x can't produce the exact dist contract (paths, no-import/export SW, d.ts), drop to its underlying OSS tools (tsdown/vitest/oxlint/oxfmt as direct devDeps) for the parts that fall short, and record which and why here.

## Todos

- [x] Rename to @jphastings/gosd across package.json, README(s), PUBLISHING.md, docs/image-injection.md, CLAUDE.md, error messages/tests that cite the subpath
- [x] pnpm workspace conversion (lockfile, packageManager pin, scripts)
- [x] Vite+ toolchain: build/test/lint/fmt via vp; dist contract preserved; typecheck trio stays tsc
- [x] CI: ci.yml js job + publish-npm.yml on pnpm; publish step stays npm CLI; --access public added
- [x] Docs + beans (gosd-xe3r checklist for the scoped name)
- [x] Gates: full js suite + Go suite + bare-node dist import check

## Summary of Changes

**Rename.** `js/packages/gosd/package.json` name is now `@jphastings/gosd`
(`publishConfig.access: "public"` kept, now load-bearing). Every source
docstring/error string, the package README, `docs/image-injection.md`, root
`README.md`, and the CLAUDE.md carve-out bullet now say
`@jphastings/gosd/downloads` and `@jphastings/gosd/downloads/service-worker.js`.
Export subpaths themselves (`./downloads`, `./downloads/service-worker.js`)
are unchanged. `js/PUBLISHING.md` rewritten to explain tags are keyed on the
package's **directory** under `js/packages/`, not its (possibly scoped) npm
name, with the bare-name 403 story recorded so future packages default to
`@jphastings/<name>`. Bean `gosd-xe3r` (publish checklist) updated to the
scoped name and the "claim the name" urgency dropped — a scoped name under
an owned account can't be squatted.

**pnpm.** Installed pnpm 11.20.0 globally (latest; the bean's "10.x" was a
guess — 11.x is actually current). Added `js/pnpm-workspace.yaml`
(`packages: ["packages/*"]`), deleted `js/package-lock.json`, added
`"packageManager": "pnpm@11.20.0"` to `js/package.json`. Root scripts now
recurse via `pnpm -r run <script>` (build/typecheck/test/test:integration/
lint/format/format:check), delegating to each package's own script. One
undeclared-dependency issue surfaced exactly as the bean warned: a clean
`pnpm install` does NOT reliably hoist `vitest` into `js/packages/gosd`'s
resolution path (an earlier warm-cache install misled initial testing) —
`vitest@4.1.10` (pinned exact, matching vite-plus's bundled version) is now
a direct devDependency of `js/packages/gosd/package.json`, confirmed via a
from-scratch `rm -rf node_modules && pnpm install`.

**Vite+.** `vite-plus@0.2.7` (verified latest 0.2.x) added as a js-root
devDependency; `pnpm exec vp --version` confirmed the bundled toolchain
(vite 8.1.5, vitest 4.1.10, oxfmt 0.60.0, oxlint 1.75.0, tsdown 0.22.14).
New `js/packages/gosd/vite.config.ts` replaces `vitest.config.ts` (deleted;
Vite+ recommends config live in `vite.config.ts`, not a separate
`vitest.config.ts`) — it carries both the `test.projects` (unit/integration)
split, unchanged in meaning, and a `pack` array with two tsdown entries:
`downloads/index` (ESM, bundled `.d.ts`) and `sw/gosd-download-sw` (IIFE, no
dts). `package.json` scripts: `build` = `rm -rf dist && vp pack` (the
explicit `rm -rf` avoids a real race — `vp pack`'s two array entries build
concurrently, so tsdown's own per-entry `clean` can't be used, as whichever
finished last would wipe what the other just wrote), `test` = `vp test
--project unit`, `test:integration` = `pnpm run genfixture && vp test
--project integration`, `lint` = `vp lint`, `format`/`format:check` = `vp fmt`
/ `vp fmt --check`, `prepack` = `rm -rf dist && vp pack` (calls the tool
directly rather than `npm run build`/`pnpm run build`, so a plain
`npm publish` works standalone regardless of which package manager installed
the deps — verified via `npm publish --dry-run`). `typecheck` is unchanged
(three `tsc --noEmit` programs); `tsconfig.typecheck.json` now includes
`vite.config.ts` instead of the deleted `vitest.config.ts`.
`.prettierrc.json`/`.prettierignore` deleted, `prettier` devDependency
removed (was already absent from the rewritten root `package.json`); the
existing root `.gitignore` (dist, node_modules, the integration fixture)
turned out to already be sufficient for oxfmt/oxlint's default ignore
behaviour (oxfmt also skips `node_modules` by default) — verified by running
`vp fmt`/`vp lint` from both the package directory and the js/ root and
confirming no generated/vendored file was ever scanned or reformatted, so no
extra ignore file was added. One real lint finding fixed: an unused `image`
destructure in `substitute.test.ts`.

**vp pack caveat (fallback clause exercised, narrowly).** `vp pack`'s `.d.ts`
generation auto-selects the `tsgo` generator whenever TypeScript 7's
native-preview compiler is installed (true here — `typescript: ^7.0.2` is the
project's pin) — that path either crashed (`generator: "tsc"`, which needs a
stable 5.x/6.x compiler API TS7 doesn't provide) or silently emitted a
garbled non-declaration `.ts` file instead of `.d.ts` (the default
auto-inferred `tsgo` path). Fixed entirely within `vp pack`'s own config
surface by pinning `dts: { generator: "oxc" }` (isolated declarations, no
tsc dependency) on the library entry — no drop to raw tsdown was needed, and
no source changes were needed to satisfy isolatedDeclarations. Also needed:
an explicit `outputOptions.entryFileNames` on the SW entry only, since
tsdown appends a `.iife`/`.umd` infix to non-ESM-format output filenames by
default (`gosd-download-sw.iife.js`), which would have broken the FROZEN
dist path; the ESM lib entry needed no such override (its default naming
already matched, and adding one broke `.d.ts` generation entirely — it
diverted the file into that same garbled-`.ts` failure mode).

**CI.** `pnpm/action-setup` pinned to `0977fd99725f1db4007ccb2928dbb4e90d06cc86`
(tag `v6.0.10`, dereferenced — resolved via
`git ls-remote --tags https://github.com/pnpm/action-setup`, cross-checked
against the floating `v6^{}` ref pointing at the same commit), `version:
11.20.0`. `ci.yml`'s `js` job and `publish-npm.yml`'s `verify`/`publish` jobs
all switched to pnpm install + `pnpm run <gate>` (added `lint` to both gate
lists); `setup-node` now uses `cache: pnpm` /
`cache-dependency-path: js/pnpm-lock.yaml`. `publish-npm.yml`'s tag-parsing
step now treats the 2nd tag segment as the package **directory**, requires
`js/packages/<dir>/package.json` to exist, and outputs both `dir` and the
manifest's real `name` (dropped the old `name == dir` equality check, since
that's now expected to differ for scoped packages). The `publish` step keeps
using the npm CLI (`npm publish --provenance --access public --tag next`)
with a comment explaining why pnpm isn't used for the actual publish
(pnpm/pnpm#9812 — no OIDC support); `smoke` job now keys everything off the
`name` output. `.gitignore` needed no changes — `pnpm-lock.yaml` is
committed by design and every generated path pnpm/vp/vitest touch was
already covered.

**Gates.** All green from a from-scratch `pnpm install`: format:check, lint,
typecheck, build, test (111), test:integration (4); the bare-node ESM import
check (29 exports); `grep -c "import\|export"` on the compiled SW script
returns 0; `npm publish --dry-run` lists exactly `dist/downloads/index.js`,
`dist/downloads/index.d.ts`(+`.map`), `dist/sw/gosd-download-sw.js`,
`README.md`, `LICENSE`, `package.json`, name `@jphastings/gosd`. Go suite
(`go test`, `go vet`, `gofmt -l .`, `golangci-lint run ./...` both darwin and
`GOOS=linux`) unaffected and green. `python3 -c "import yaml; ..."` confirms
both workflow files still parse.
