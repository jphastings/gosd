---
# gosd-my8e
title: 'Report metadata: --support-url, app name/version, board display names'
status: todo
type: feature
priority: high
created_at: 2026-08-11T10:11:16Z
updated_at: 2026-08-11T10:24:39Z
parent: gosd-47z3
---

Part of epic gosd-47z3. Blocks every other child: the report's frontmatter
and its "visit <support_url>" line have no data source in the codebase today.

## What's missing

- **`device: Raspberry Pi Zero 2W (pi-zero-2w)`** — `internal/boards.Board`
  carries only the id (`pi-zero-2w`, `pi-3b`, …). There is no human-readable
  name anywhere in Go; COMPATIBILITY.md has them in prose only.
- **`image: myapp 0.1.0 #a1b2c3d4`** — `internal/initcfg.Config` has
  `Identity` (a deterministic content hash, with `ShortIdentity()` already
  built for exactly this "tell builds apart at a glance" job) and
  `BuildTimestamp`, but no app name and no app version. `Hostname` is a bad
  proxy for the app name: it defaults to the main package's basename but is
  overridable by `--hostname`, gosd.toml and cloud-init, so on a
  user-renamed device it would name the wrong thing.
- **`<support_url>`** — nothing like it exists. Without it the report's "The
  fix" section has nowhere to send someone.

## Todos

- [ ] `boards.Board.DisplayName` (e.g. "Raspberry Pi Zero 2W", "Radxa ROCK
      4SE"), populated for every registered board including `qemu-virt`.
      Assert in the board-registration test that no board leaves it empty
- [ ] `gosd build --support-url <url>`: validated as an absolute http(s) URL
      at build time (a broken link in a crash report is worse than none),
      baked to `config.json` as `supportURL`
- [ ] `gosd build --app-version <string>`: free-form, baked as `appVersion`.
      Optional — when unset, the report's `image:` line falls back to
      `myapp #a1b2c3d4` using `ShortIdentity()` alone
- [ ] Bake the app name (`appName`) from the main package's basename — the
      same source `--hostname`'s default already uses, resolved once at
      build time so a later hostname override can't change it
- [ ] Each new field optional in `initcfg.Config`, absent-safe the way
      `Identity`/`BuildTimestamp` already are: an image built before the
      field existed must render a report with the field omitted, never a
      wrong value
- [ ] These are developer-set, not operator-set: config.json only, no
      gosd.toml key and no `GOSD_*` override. Note it in docs/gosd.toml.md
      if that file enumerates what is and isn't overridable
- [ ] Not on-card ABI — none of these participate in the adoption gate, so
      changing `--support-url` between releases must NOT reformat anyone's
      data partition. Confirm against `docs/design/upgrade-path.md`'s list
- [ ] `gosd build --help` text, docs, and a fixture-driven integration test
      that reads the built image back and asserts config.json's contents
      (network-tripwire pattern, `cmd/gosd/build_integration_test.go`)

## LOCKED: --app-version is an explicit flag (JP, 2026-08-11)

A free-form string GoSD never interprets. Deriving it from the app's VCS
state via `debug.ReadBuildInfo` was considered and rejected: gosd compiles
the user's app on their machine, where the VCS state may be dirty or absent,
and a wrong version in a crash report is worse than no version at all.
