---
# gosd-cn4p
title: 'Config tree build side: defaults dir, app overlay, manifest config entries, TypeScript client'
status: completed
type: feature
priority: normal
created_at: 2026-08-13T15:39:18Z
updated_at: 2026-08-13T17:02:54Z
parent: gosd-rw6n
---

The build half of epic gosd-rw6n (which holds all locked decisions), plus the
TypeScript client, in ONE PR: CI regenerates the js integration fixture from
`internal/cmd/injectfixture` and runs the TS integration test against it, so
the manifest and the client cannot move separately (proven on gosd-ypyz and
again on #271).

gosd.toml rendering is NOT touched in this bean — the tree is written to the
boot partition alongside it, so gosd-init and the qemu CI jobs stay green
until the read-side bean switches over.

## Todos — Go

- [x] gosd's default `config/` tree checked into this repo: values +
      `.explain.md` for `hostname`, `wifi/ssid`, `wifi/passphrase`,
      `data_flush`, `ingress/cloudflared/*`, `ingress/tailscale-funnel/*`;
      group `explain.md` files; `env/` ships a group explain.md only
- [x] `gosd build --config-dir <dir>` overlay (default: `config/` adjacent to
      the app's main package when present); per-file overlay, app wins,
      explanations inherited
- [x] Build-gate validation: explain.md required per value (own or
      inherited); reserved-name and junk refusal (`.new`, `.unused`,
      `explain.md`, leading-dot, `._*`, `Thumbs.db`, `desktop.ini`);
      case-insensitive collision refusal; `env/GOSD_*` refusal; env-name
      shape refusal
- [x] Per-board feature pruning: a feature's directory written only when the
      board supports it AND the build enables it
- [x] Padding/reservation: pad value files with trailing newlines to
      >=256 bytes; a larger shipped file reserves its own size; explain.md
      unpadded
- [x] Per-value-file sha256 map into config.json (initcfg)
- [x] Manifest: top-level `config` array `{path, size, sha256, ranges,
      value}` (value = trimmed pristine); REMOVE the unshipped
      `--env-placeholder` machinery — the flag, `gosdtoml.RenderWithReservedEnv`,
      `image.RangeRequest`/`spanOfRanges` (ReportRanges reverts to
      `[]string`), and the manifest `env` key
- [x] `internal/cmd/injectfixture` builds a real tree; fixture manifest
      carries config entries
- [x] Remove `--env`/`--env-file`/`--hostname`/`--wifi-ssid`/`--wifi-password`
      (per the epic's flags decision) and their plumbing where it is
      build-side only; anything gosd-init still reads stays until the
      read-side bean

## Todos — TypeScript

- [x] Manifest parsing: `config` array; delete `EnvInfo`; each config entry
      joins the existing region machinery keyed by its full path
- [x] `config: Record<string, string>` option on `withPlaceholders` and
      `resumeDownload`: keys without the `config/` prefix, values padded with
      trailing newlines to the region size; pre-download refusals (unknown
      path listing what exists, too-long value, `env/GOSD_*`, image with no
      config entries)
- [x] Delete the `env` option, `renderEnvBody`, `env.ts` and all TOML
      handling
- [x] Integration test: inject `env/<NAME>` and `ingress/cloudflared/token`
      values into the Go-built fixture; FAT-level readback; sibling files
      byte-identical
- [x] README: the `config` option; no mention of the previous format

## Todos — gates

- [x] `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run
      ./...` and `GOOS=linux golangci-lint run ./...`
- [x] `cd js && pnpm install --frozen-lockfile && pnpm run format:check &&
      pnpm run lint && pnpm run typecheck && pnpm run build && pnpm test &&
      pnpm run test:integration`

## Summary of Changes

The build half of the config tree, plus the TypeScript client, in one PR.

**`internal/configtree`** is the new home of the format: gosd's own defaults
are a real `defaults/` directory embedded with `go:embed`, `Build(overlayDir,
Features)` merges an app's `--config-dir` over them file by file, validates,
prunes to the features the image carries, and pads every value file to its
reservation. `Tree.BootFiles()` keys files by their path on the card
(`config/...`), `Tree.Digests()` is what config.json bakes, and
`TrimValue` is the one definition of what "the value" is - the read side
(gosd-ypkv) should use it rather than trimming its own way.

- **Defaults tree** (`internal/configtree/defaults/`): `hostname`,
  `wifi/{ssid,passphrase}`, `data_flush`, `ingress/cloudflared/*`,
  `ingress/tailscale-funnel/*`, all shipped EMPTY (unset - config.json's
  baked value is the per-field fallback), each with a plain-language
  `<name>.explain.md`; group `explain.md` for the root, `wifi/`, `env/`
  (which ships only that) and each ingress agent.
  `ingress/cloudflared/token` ships as 1024 newlines: padding is the
  reservation, and a Cloudflare tunnel token doesn't reliably fit in 256
  bytes.
- **Build gates**: an undocumented value, an orphan `<name>.explain.md`, a
  reserved name (`.new`/`.unused` suffix, leading dot, `._*`, `Thumbs.db`,
  `desktop.ini`), a case-insensitive collision, a value that is also a
  directory, `env/GOSD_*`, and an env name no environment could carry are
  each refused with an actionable error naming the file on disk.
- **Pruning**: `ingress/cloudflared/` and `ingress/tailscale-funnel/` are
  written only when that agent is baked in AND the board's architecture can
  run it (`ingressFeaturesFor` in cmd/gosd), and `ingress/` itself goes when
  neither is.
- **Manifest**: top-level `config` array of
  `{path, size, sha256, ranges, value}`, hashed from the bytes gosd wrote
  (so a mis-published range fails a client's pristine check rather than
  hiding behind it). The `--env-placeholder` machinery is gone:
  the flag, `gosdtoml.RenderWithReservedEnv`/`Span`,
  `image.RangeRequest`/`spanOfRanges` (`ReportRanges` is `[]string` again),
  and the manifest's `env` key.
- **Flags**: `--config-dir <dir>` added to `gosd build` and `gosd run`
  (default: a `config/` directory beside the app's main package);
  `--env`, `--env-file`, `--hostname`, `--wifi-ssid`, `--wifi-pass` and
  `--env-placeholder` removed, along with `boards.BuildConfig`'s now-dead
  `Env`/`WifiSSID`/`WifiPassword`/`HostnameExplicit` fields. `gosd.toml` is
  still rendered, now always as its commented-out examples.
- **TypeScript**: `config: Record<string, string>` on `withPlaceholders` and
  `resumeDownload`, keyed without the `config/` prefix; `ConfigInfo` joins
  the existing region machinery under `configRegionKey(path)`; `env.ts`,
  `renderEnvBody`, `padEnv`, `ENV_REGION_KEY` and all TOML handling deleted;
  new `GosdUnknownConfigError`. The integration test injects `env/API_TOKEN`
  and `ingress/cloudflared/token` into the Go-built fixture and proves the
  rest of the image is byte-identical.

### Decisions taken here (not relitigated, but worth knowing)

- **Every build now writes an `<image>.inject.json`**, not just
  `--placeholder` builds: every image has settings, so every image is
  injectable.
- **The manifest's `config[].path` has no `config/` prefix**, matching the
  TypeScript option's keys exactly; the region key the client uses
  internally *is* prefixed, so it can never collide with a placeholder path.
- **`.DS_Store` and friends are a build refusal**, per the locked decision.
  Worth watching: a Mac developer whose `config/` directory picks one up
  gets a failed build until they delete it. The error says exactly that,
  but if it turns out to be a nuisance in practice the alternative
  (ignore-with-warning) is a one-line change in `checkName`.
- **Docs**: this PR fixed the docs its own contract changed
  (`docs/image-injection.md`, the release notes, COMPATIBILITY.md) and
  removed every reference to a flag it deleted. It did NOT rewrite
  `docs/gosd.toml.md`/`docs/runtime.md` around the tree - that is gosd-fdt2,
  written as if the tree always existed, once the read side and the store
  exist to describe.
