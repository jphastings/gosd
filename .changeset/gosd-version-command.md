---
gosd: minor
---

#### `gosd version` says which board artifacts your images will be built from

`gosd` had no way to report its own version, and no way at all to answer the
question that decides whether an image boots: which release of board kernels
and bootloaders it downloads.

```console
$ gosd version
gosd:      v0.6.2
artifacts: v0.10.2
go:        go1.26.5
```

`gosd --version` prints the same. A binary built from a checkout reports its
commit and whether the tree was modified, so "it works on my machine" is
answerable. When a board boots with one `gosd` and not another, the artifacts
line is usually where they differ.
