---
gosd: minor
---

#### `gosd cache` inspects and clears the CLI's on-disk caches

`gosd build`/`run`/`build-kernel`/`build-external` already auto-prune their
pinned-download caches to the current pin after every successful run, and the
durable `build-kernel`/`build-external` state directory now keeps only its 8
most recently used entries — everyday growth was already bounded. `gosd
cache` adds manual visibility and control on top of that:

- `gosd cache dir` prints the path of every cache location.
- `gosd cache size` reports how much disk space each one is using, and a
  total.
- `gosd cache clean` deletes the pinned-download caches (board artifacts, the
  CA bundle, ingress binaries, kernel firmware) — always safe, since every
  one is a sha256-verified download the next build/run simply re-fetches.

`gosd cache clean` deliberately leaves the `build-kernel`/`build-external`
state alone by default: each of its entries costs 20-75 minutes of container
build time to reproduce. Pass `--builds` to also clear it.
