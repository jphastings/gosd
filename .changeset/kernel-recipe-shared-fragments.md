---
gosd: minor
---

#### `gosd-kernel.toml` recipe variants can now share a fragment

A `[kernel.<board-id>]` section's `fragment` key now has a list-typed
sibling, `fragments`: an ordered list of Kconfig fragment paths, merged in
order, each after the last, exactly the way a single `fragment` is already
merged after GoSD's own board fragment. Two recipe variants of the same
board — a cheap one and one that additionally enables DRM, say — can now
list a shared fragment first and their own variant-specific fragment last,
instead of each carrying a full copy of the shared content. `fragment` and
`fragments` are mutually exclusive on the same board. See
[the custom-kernel recipe docs](docs/custom-kernels.md) for the full syntax.
