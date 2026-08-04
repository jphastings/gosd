---
# gosd-g7n4
title: Rename npm package to @jphastings/gosd; migrate js/ to pnpm + Vite+
status: in-progress
type: task
created_at: 2026-08-04T12:24:06Z
updated_at: 2026-08-04T12:24:06Z
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

- [ ] Rename to @jphastings/gosd across package.json, README(s), PUBLISHING.md, docs/image-injection.md, CLAUDE.md, error messages/tests that cite the subpath
- [ ] pnpm workspace conversion (lockfile, packageManager pin, scripts)
- [ ] Vite+ toolchain: build/test/lint/fmt via vp; dist contract preserved; typecheck trio stays tsc
- [ ] CI: ci.yml js job + publish-npm.yml on pnpm; publish step stays npm CLI; --access public added
- [ ] Docs + beans (gosd-xe3r checklist for the scoped name)
- [ ] Gates: full js suite + Go suite + bare-node dist import check
