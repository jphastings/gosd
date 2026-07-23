---
# gosd-6h1x
title: gosd build . derives hostname 'app' instead of the directory name
status: todo
type: bug
created_at: 2026-07-23T13:15:22Z
updated_at: 2026-07-23T13:15:22Z
---

Found during rock-4se bring-up (gosd-sz6p, 2026-07-23): an app built from inside its directory with 'gosd build .' booted advertising app.local, not <dirname>.local.

Mechanism: cmd/gosd/build.go derives appName via naming.Sanitize(filepath.Base(filepath.Clean(pkgPath))). With pkgPath '.', Base(Clean('.')) = '.', which Sanitize reduces to '' and falls back to 'app'. The README quickstart's canonical invocation is exactly 'gosd build .', so end users hit this by default. Also affects default output naming (<appname>-<board>.img → app-<board>.img) via resolveOutputs.

Fix: resolve pkgPath to an absolute path (filepath.Abs) before taking the basename, so 'gosd build .' in ~/myapp yields hostname 'myapp'. Behavioral test: build with pkgPath '.' from a temp dir named e.g. 'widget-3' and assert hostname/output name. Locked decision context: default hostname is 'the sanitized basename of the app's main package' — this is a bug in that derivation, not a decision change.
