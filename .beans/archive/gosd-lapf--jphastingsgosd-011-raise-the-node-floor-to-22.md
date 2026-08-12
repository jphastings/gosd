---
# gosd-lapf
title: '@jphastings/gosd 0.1.1: raise the Node floor to 22'
status: completed
type: task
created_at: 2026-08-04T13:34:14Z
updated_at: 2026-08-04T13:34:14Z
---

Bump the package's engines floor from >=20 to >=22 and release as 0.1.1 — the first release through the staged publish pipeline. Node 20 support forced an awkward CI split (pnpm 11 needs >=22.13 for node:sqlite; vite-plus 0.2.x calls Promise.withResolvers, Node 22+, despite claiming ^20.19 engines): the toolchain ran on 22/24 while a dedicated js (node 20 runtime) job proved the engines claim via vitest directly with a vite-plus-free config. Raising the floor to 22 makes the toolchain's floor and the package's floor agree, so that job and vitest.node20.config.ts are deleted, and every node-20 caveat comment goes with them. Node 20 reaches end-of-life 2026-04-30 (already past), so nothing supported is dropped.

## Summary of Changes

- `js/packages/gosd/package.json`: `engines.node` `>=20` → `>=22`; version `0.1.0` → `0.1.1` (the first release through the staged publish pipeline).
- Deleted `.github/workflows/ci.yml`'s `js-node20-runtime` job and `js/packages/gosd/vitest.node20.config.ts` — both existed solely to prove the node-20 engines claim the toolchain itself couldn't test (pnpm 11 needs node:sqlite ≥22.13; vite-plus 0.2.x calls Promise.withResolvers). With the floor at 22, the js job's node 22 matrix leg genuinely tests the oldest supported runtime.
- Comment cleanups: ci.yml js-job header now states the aligned floors; publish-npm.yml's two stale "keeping node 20 a real test" standalone-pnpm comments rewritten.
- Package README: core API "plain Node (20+)" → "(22+)".
- Node 20 was already past end-of-life (2026-04-30), so no supported runtime loses coverage.

Release: after merge, tag `npm/gosd/v0.1.1` on main → approve the npm-publish environment → verify → `npm dist-tag add @jphastings/gosd@0.1.1 latest`.
