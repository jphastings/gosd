---
# gosd-hxz9
title: Release @jphastings/gosd 0.2.0 (injectable app settings)
status: completed
type: task
priority: normal
created_at: 2026-08-12T14:51:58Z
updated_at: 2026-08-12T14:57:58Z
---

Step 1 of `js/PUBLISHING.md`'s release procedure for the npm package: land a
PR bumping `version` in `js/packages/gosd/package.json`, so the
`npm/gosd/v<version>` tag JP pushes afterwards agrees with the manifest (the
workflow refuses a tag that doesn't).

**0.1.1 -> 0.2.0.** Additive, no breaking change: `withPlaceholders` gained an
`options.env` for the reserved `[env]` region (bean gosd-ypyz), every existing
call keeps working, and manifests without an `env` key parse exactly as before.
Pre-1.0, so a feature moves the minor.

PR #269 merged while this was in flight, so this targets main directly rather
than stacking on it.

## Todos

- [x] Bump `js/packages/gosd/package.json` to 0.2.0
- [x] `js/` gates: install, format:check, lint, typecheck, build, test,
      test:integration
- [x] Go gates (the fixture generator is Go module code)

## After the merge (JP)

```sh
git tag npm/gosd/v0.2.0 && git push origin npm/gosd/v0.2.0
```

then approve the **npm-publish** environment run, and promote off `next` by
hand once it looks right. Never publish to `latest` from CI.


## Summary of Changes

- `js/packages/gosd/package.json`: `0.1.1` -> `0.2.0`, the only change. All
  `js/` gates and the Go gates re-run against it.
- The publish itself is JP's: tag `npm/gosd/v0.2.0` on main once this and
  PR #269 have landed, approve the npm-publish environment run, then promote
  off `next` by hand.
