---
# gosd-my8e
title: 'Report metadata: --support-url, app name/version, board display names'
status: completed
type: feature
priority: high
created_at: 2026-08-11T10:11:16Z
updated_at: 2026-08-11T10:51:55Z
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

- [x] `boards.Board.DisplayName` (e.g. "Raspberry Pi Zero 2W", "Radxa ROCK
      4SE"), populated for every registered board including `qemu-virt`.
      Assert in the board-registration test that no board leaves it empty
- [x] `gosd build --support-url <url>`: validated as an absolute http(s) URL
      at build time (a broken link in a crash report is worse than none),
      baked to `config.json` as `supportURL`
- [x] `gosd build --app-version <string>`: free-form, baked as `appVersion`.
      Optional — when unset, the report's `image:` line falls back to
      `myapp #a1b2c3d4` using `ShortIdentity()` alone
- [x] Bake the app name (`appName`) from the main package's basename — the
      same source `--hostname`'s default already uses, resolved once at
      build time so a later hostname override can't change it
- [x] Each new field optional in `initcfg.Config`, absent-safe the way
      `Identity`/`BuildTimestamp` already are: an image built before the
      field existed must render a report with the field omitted, never a
      wrong value
- [x] These are developer-set, not operator-set: config.json only, no
      gosd.toml key and no `GOSD_*` override. Note it in docs/gosd.toml.md
      if that file enumerates what is and isn't overridable
- [x] Not on-card ABI — none of these participate in the adoption gate, so
      changing `--support-url` between releases must NOT reformat anyone's
      data partition. Confirm against `docs/design/upgrade-path.md`'s list
- [x] `gosd build --help` text, docs, and a fixture-driven integration test
      that reads the built image back and asserts config.json's contents
      (network-tripwire pattern, `cmd/gosd/build_integration_test.go`)

## LOCKED: --app-version is an explicit flag (JP, 2026-08-11)

A free-form string GoSD never interprets. Deriving it from the app's VCS
state via `debug.ReadBuildInfo` was considered and rejected: gosd compiles
the user's app on their machine, where the VCS state may be dirty or absent,
and a wrong version in a crash report is worse than no version at all.

## Summary of Changes

- `boards.Board` gained `DisplayName() string`, implemented for all 8
  registered boards (matching COMPATIBILITY.md's prose, e.g. "Raspberry Pi
  Zero 2W", "Radxa ROCK 4SE"; qemu-virt got "QEMU virt" since every
  registered board must return non-empty). `cmd/gosd`'s
  `TestEveryRegisteredBoardHasADisplayName` asserts none is empty, across
  `boards.All()` plus qemu-virt (the one internal-only board).
- `gosd build --support-url <url>` (optional; validated at build time as an
  absolute http(s) URL via the new `parseSupportURL`, refusing before any
  image is written) and `gosd build --app-version <string>` (optional,
  free-form, never interpreted) are new flags, baked into `config.json` as
  `supportURL`/`appVersion`.
- The app name (already computed once per build via `deriveAppName`, the
  same source `--hostname`'s default uses) is now also baked into
  `config.json` as `appName`, independent of any later `--hostname`/
  gosd.toml/cloud-init override.
- All three new fields live on `internal/initcfg.Config`
  (`AppName`/`AppVersion`/`SupportURL`), each `,omitempty` and documented as
  optional/absent-safe exactly like `Identity`/`BuildTimestamp`. They flow
  through new `pipeline.Options` fields (`AppName`/`AppVersion`/
  `SupportURL`), set only by `cmd/gosd build.go` (not `run.go` - see the
  decision note below).
- Config.json remains entirely excluded from `ComputeIdentity`'s hashed
  payload (unchanged), so the new fields never move the image identity and
  play no part in the data-partition adoption gate (confirmed against
  `docs/design/upgrade-path.md`'s four-thing gate, which never touches
  config.json) - proven by `TestBuildIdentityUnaffectedByReportMetadata`.
  `internal/initcfg/identity.go`'s docstring was updated to name the new
  fields alongside the existing DataExpand/DataFlush example.
- `docs/gosd.toml.md` gained a note that `--support-url`/`--app-version`/the
  baked app name are config.json-only developer metadata with no gosd.toml
  key and no `GOSD_*` override.
- Tests: `cmd/gosd/build_test.go` (parseSupportURL unit tests,
  DisplayName registration test), `cmd/gosd/build_integration_test.go`
  (fixture-driven, network-tripwire-covered: bakes-in, optional-by-default,
  invalid-URL refusal, identity-unaffected), `internal/initcfg/config_test.go`
  (ParseConfig round-trip for the three new fields).

## Decisions not covered by the bean

- **`gosd run` was left untouched.** It has no `--support-url`/
  `--app-version` flags and its `pipeline.Options` doesn't set
  `AppName`/`AppVersion`/`SupportURL` - consistent with its existing
  "fast inner-loop, never a shipping image" scope (it already omits
  `--data-filesystem` and other build-only flags). `AppName` stays empty in
  a `gosd run` image's config.json; `TestBuildAndRunProduceIdenticalInitramfsContent`
  already exempts config.json from its byte-for-byte comparison, so this
  doesn't break build/run parity.
- **`DisplayName` was not baked into config.json.** The bean's todo list
  only asked for the `boards.Board.DisplayName` method, its per-board
  implementations, and a registration test - not a config.json field. Since
  `gosd-init` doesn't import `internal/boards` (it's a heavy, CLI-only
  package pulling in go-diskfs etc.), wiring the device: line's human name
  into the on-device crash report will need its own config.json field (or
  another mechanism) decided by whichever bean implements the actual
  LAST_FATAL_ERROR.md renderer (gosd-pun9 per the epic). Flagging this so
  it isn't missed when that bean starts.
