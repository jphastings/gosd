---
# gosd-m09v
title: 'Card input is unbounded: an oversized cloud-init seed OOMs PID 1, and the config tree has no whole-tree ceiling'
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:13:13Z
updated_at: 2026-08-20T05:36:19Z
---

**Re-scoped 2026-08-20.** Filed against `internal/gosdtoml`, which no longer
exists: the per-attribute config tree replaced `gosd.toml` wholesale (epic
`gosd-rw6n`). The stack overflow specifically is gone; the class of defect
is not. This body describes the surface as it is today, with the
reproductions actually run against it.

## What died with gosd.toml

`BurntSushi/toml`'s unbounded recursive descent was the whole mechanism of
the original report, and nothing on the card reaches a recursive parser any
more:

- The config tree has **no parser at all** — one value per file,
  newline-trimmed (`internal/configtree`, `cmd/gosd-init/internal/cardconfig`).
- `config.json` is decoded with `encoding/json`, and is baked into the
  initramfs at `/etc/gosd/config.json` — not on the card, so not
  attacker-writable without rebuilding the image.
- cloud-init's seed is the one card input that still goes through a parser,
  and `gopkg.in/yaml.v3` **has its own depth limit**. Measured on this
  repo's pin, `yaml.Unmarshal` of `strings.Repeat("[", n) + strings.Repeat("]", n)`:

  | n | file size | result |
  |---|---|---|
  | 10,000 | 20 KB | parses, `err=<nil>` |
  | 100,000 | 200 KB | `yaml: exceeded max depth of 10000` |
  | 1,000,000 | 2 MB | same |
  | 3,000,000 | 6 MB | same |

  No crash at any depth. **No card input can stack-overflow PID 1 today.**

Also already fixed since filing: `cardconfig.readValue` caps a single
setting file at `MaxValueBytes` (64 KiB) and refuses anything that isn't
`Mode().IsRegular()`, so the huge-file, symlink, device-node and
FIFO-that-blocks-forever variants are all closed for the config tree.
`internal/secretreg` bounds its /run file at `MaxTotalBytes`.

## What is still real

### 1. cloud-init's seed is read unbounded, and YAML amplifies ~40x

`internal/provision.readOptional` was a bare `os.ReadFile` on `user-data`
and `network-config` — files on the **FAT boot partition**, so writable by
anyone who can plug the card into a computer. Measured on this repo's pin:

    input 4,000,000 bytes -> heap grew 165,095,696 bytes (41.3x)

for `strings.Repeat("- a\n", 1_000_000)` unmarshalled into a `yaml.Node`.
So a ~12 MB `user-data` becomes ~500 MB of nodes on a board with 512 MB of
RAM and its entire root filesystem in RAM. Linux will not kill PID 1 to
reclaim memory — it panics — and the file is still on the card on the next
boot. Same permanent boot loop as the original report, same one-file
attack, different file. **This is the surviving gosd-m09v.**

`readOptional` also opened the file before checking what it was, so a named
pipe called `user-data` would block the read forever: a boot that never ends
and never says why.

### 2. The config tree is capped per file but not in total

`cardconfig.Read` walked and retained **every** file it found. A 256 MiB
boot partition (the `--boot-size` default) filled with settings, each one
comfortably under `MaxValueBytes`, is a couple of hundred megabytes read
into a map on a RAM rootfs — the same OOM, reached by volume rather than by
one big file. Nothing bounded the walk's depth either, and the walk is
recursive over directories on a card somebody else wrote.

## Fix

- `internal/provision`: `MaxSeedBytes` = 256 KiB, refused before the file is
  opened, along with anything that isn't an ordinary file. Imager 2.0.10's
  own seeds — the captured fixtures in `internal/provision/testdata` — are
  267 to 475 bytes, so this is three orders of magnitude of headroom, and it
  caps the yaml.Node tree at ~10 MiB.
- `cardconfig`: `MaxTreeBytes` = 1 MiB across the whole tree, each setting
  charged at least its reservation (`configtree.MinValueBytes`, 256), so it
  bounds how many settings there are as well as how big they get — room for
  four thousand, where gosd ships eleven. Plus `MaxDepth` = 8, against a
  crafted FAT directory entry pointing back at an ancestor.

Reaching either ceiling logs one actionable line and leaves the settings not
read at their baked values, which is the same outcome as a card that never
carried them.

## Deliberately not done

`debug.SetMaxStack` — process-global, and with no recursive parser left on
any card path there is nothing for it to protect.

## Todos

- [x] Establish which of the original mechanisms survive the config-tree rewrite
- [x] Size-cap `provision.readOptional` for user-data and network-config
- [x] Refuse a seed that isn't an ordinary file, before opening it
- [x] Bound the config tree in total, not only per file
- [x] Bound the config tree's walk depth
- [x] Test: an oversized seed is ignored with a logged, actionable message and boot continues on baked defaults
- [x] Test: a named pipe in place of a seed doesn't block the boot
- [x] Test: a tree of individually-legal settings stops at the ceiling
- [x] Confirm no card input reaches a recursive parser (measured: yaml.v3 caps depth at 10,000)

## Summary of Changes

Re-scoped from a dead file to the current surface, then fixed the two
unbounded card inputs that survived it. `internal/provision` gained
`MaxSeedBytes` plus a regular-file check ahead of the open;
`cmd/gosd-init/internal/cardconfig` gained `MaxTreeBytes` (charged per
setting at its reservation floor) and `MaxDepth`. Each constant's value is
justified in the code against a measured fact — Imager's real seed sizes,
yaml.v3's ~41x node amplification, `configtree.MinValueBytes` — rather than
picked. The stack-overflow half of the original report is closed as
obsolete: yaml.v3 enforces its own depth limit, and no other card input is
parsed recursively.
