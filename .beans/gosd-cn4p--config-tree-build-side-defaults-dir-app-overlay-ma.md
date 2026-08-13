---
# gosd-cn4p
title: 'Config tree build side: defaults dir, app overlay, manifest config entries, TypeScript client'
status: todo
type: feature
created_at: 2026-08-13T15:39:18Z
updated_at: 2026-08-13T15:39:18Z
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

- [ ] gosd's default `config/` tree checked into this repo: values +
      `.explain.md` for `hostname`, `wifi/ssid`, `wifi/passphrase`,
      `data_flush`, `ingress/cloudflared/*`, `ingress/tailscale-funnel/*`;
      group `explain.md` files; `env/` ships a group explain.md only
- [ ] `gosd build --config-dir <dir>` overlay (default: `config/` adjacent to
      the app's main package when present); per-file overlay, app wins,
      explanations inherited
- [ ] Build-gate validation: explain.md required per value (own or
      inherited); reserved-name and junk refusal (`.new`, `.unused`,
      `explain.md`, leading-dot, `._*`, `Thumbs.db`, `desktop.ini`);
      case-insensitive collision refusal; `env/GOSD_*` refusal; env-name
      shape refusal
- [ ] Per-board feature pruning: a feature's directory written only when the
      board supports it AND the build enables it
- [ ] Padding/reservation: pad value files with trailing newlines to
      >=256 bytes; a larger shipped file reserves its own size; explain.md
      unpadded
- [ ] Per-value-file sha256 map into config.json (initcfg)
- [ ] Manifest: top-level `config` array `{path, size, sha256, ranges,
      value}` (value = trimmed pristine); REMOVE the unshipped
      `--env-placeholder` machinery — the flag, `gosdtoml.RenderWithReservedEnv`,
      `image.RangeRequest`/`spanOfRanges` (ReportRanges reverts to
      `[]string`), and the manifest `env` key
- [ ] `internal/cmd/injectfixture` builds a real tree; fixture manifest
      carries config entries
- [ ] Remove `--env`/`--env-file`/`--hostname`/`--wifi-ssid`/`--wifi-password`
      (per the epic's flags decision) and their plumbing where it is
      build-side only; anything gosd-init still reads stays until the
      read-side bean

## Todos — TypeScript

- [ ] Manifest parsing: `config` array; delete `EnvInfo`; each config entry
      joins the existing region machinery keyed by its full path
- [ ] `config: Record<string, string>` option on `withPlaceholders` and
      `resumeDownload`: keys without the `config/` prefix, values padded with
      trailing newlines to the region size; pre-download refusals (unknown
      path listing what exists, too-long value, `env/GOSD_*`, image with no
      config entries)
- [ ] Delete the `env` option, `renderEnvBody`, `env.ts` and all TOML
      handling
- [ ] Integration test: inject `env/<NAME>` and `ingress/cloudflared/token`
      values into the Go-built fixture; FAT-level readback; sibling files
      byte-identical
- [ ] README: the `config` option; no mention of the previous format

## Todos — gates

- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run
      ./...` and `GOOS=linux golangci-lint run ./...`
- [ ] `cd js && pnpm install --frozen-lockfile && pnpm run format:check &&
      pnpm run lint && pnpm run typecheck && pnpm run build && pnpm test &&
      pnpm run test:integration`
