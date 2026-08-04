---
# gosd-xe3r
title: Publish the gosd npm package
status: todo
type: task
created_at: 2026-08-04T10:10:22Z
updated_at: 2026-08-04T10:10:22Z
---

Follow-up to gosd-hcyn: publish js/packages/gosd to npm. JP action: register/claim the bare 'gosd' package name (verified unregistered 2026-08-04; fallback @jphastings/gosd — a one-line name change, the gosd/downloads subpath import shape survives either way). First release manual: cd js/packages/gosd && npm publish (prepack runs the build). Decide on a version tag/automation story later — consider a GitHub Actions publish workflow keyed to js/vX.Y.Z tags once the package has settled.
