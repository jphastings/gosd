---
gosd: patch
---

#### `gosd build-kernel` now refuses a fuzzy or skipped device-tree patch instead of applying it silently

Every `.patch` file `gosd build-kernel` applies — GoSD's own board patches and
a developer overlay's `patches` — now applies with `patch -p1 --fuzz=0`
instead of `--forward`. A hunk against a freshly cloned, exactly-pinned
kernel source tree can never legitimately need fuzzy context matching or be
"already applied"; if either happens, something (most likely a kernel-tag
bump shifting nearby source lines) has silently changed what the patch
actually does. The build now fails loudly, naming the offending patch,
instead of shipping a kernel that silently missed the peripheral enablement
the patch was meant to provide. Write overlay patches against the pinned
kernel tag your board's `internal/kernelspec` entry uses, not a nearby one.
