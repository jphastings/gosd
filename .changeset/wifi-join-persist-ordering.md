---
gosd: patch
---

#### `wifi.Join` with `Persist` no longer returns before the network is saved

`wifi.Join` could return as soon as the join succeeded while gosd-init was still writing the network to the card, so an app that told its user "saved, you can unplug" could lose it. With `Options{Persist: true}`, `Join` now returns only once the credentials have been written.
