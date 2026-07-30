---
# gosd-vk8s
title: 'gosd-kernel.toml: let recipe variants share one fragment (a list, or an include)'
status: todo
type: task
priority: low
created_at: 2026-07-30T09:45:00Z
updated_at: 2026-07-30T09:45:00Z
---

Found while writing the Rockchip audio recipes (bean `gosd-lrxz`).

A `[kernel.<board>]` entry takes exactly one `fragment`, so two variants of the
same feature for the same board cannot share any part of their Kconfig. The
audio recipes hit this hard: `rock-4se-analog.fragment`,
`rock-4se-hdmi.fragment` and `radxa-zero-3e-hdmi.fragment` differ by roughly a
dozen lines each, and then carry an identical ~130-line deny-list, three times.
The deny-list is generated from the pinned defconfig (each fragment says how to
regenerate it), so the copies are consistent today — but a kernel bump means
editing three files, and a reviewer has to diff them to see they match.

Two obvious shapes, either would do:

- `fragment = ["common.fragment", "hdmi.fragment"]` — accept a list and merge
  in order. Cheap: the overlay already concatenates GoSD's fragment and the
  developer's, so this is one more level of the same thing.
- An `include`/`extends` key on the fragment or the board table.

Prefer the list: no new file format, and the merge order is visible at the call
site. `internal/kernelconfig`'s `Fragment string` becomes a `[]string`-or-string
union (TOML makes that ugly; a separate `fragments` key that is mutually
exclusive with `fragment` may be cleaner), and `internal/kernelbuild`'s overlay
concatenates them before writing `overlay-fragment.config`. The cache key must
hash all of them, in order.

Not urgent — three duplicated deny-lists are ugly, not wrong — but the next
feature with a cheap and an expensive variant will want it too.
