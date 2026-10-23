---
# gosd-x5c7
title: Upgrade to Go 1.27.3 and latest dependencies for the crypto/x509 CA removal
status: completed
type: task
priority: high
created_at: 2026-10-23T01:36:29Z
updated_at: 2026-10-23T01:36:29Z
---

Go 1.27.3 / 1.26.10 (2026-10-20) are security releases whose only change is
to crypto/x509: chains through eight intermediates from one CA (reported as
eMudhra, after Mozilla distrusted the same eight over compromised keys) are
now rejected. The fix only reaches a binary when that binary is rebuilt with
a fixed toolchain, so every gosd-built image — gosd-init, cloudflared /
tsfunnel shims, and the app itself — needs a toolchain at or past it.

- [x] go.mod `go` directive → 1.27.3 (also tailscale.com v1.104.2's own floor,
      so it stays dependency-derived per gosd-jm2v)
- [x] Every direct dependency to latest (`go get -u`), go.sum re-tidied
- [x] `internal/build.MinGoVersion` tracks the directive
- [x] go-diskfs v1.9.3 → v1.9.4 (forced by tailscale.com) checked against
      gosd-qvjs's warning
- [x] pion/mdns v2.2 deprecation of `mdns.Server` migrated
- [x] nix flake: nixpkgs bumped, Go 1.27 selected, vendorHash updated
- [x] CI golangci-lint bumped and built from source with the go.mod toolchain

## Summary of Changes

**Toolchain.** `go 1.27.3` in go.mod. setup-go reads it via
`go-version-file`, and `GOTOOLCHAIN=auto` fetches it for any developer whose
local `go` is older, so gosd-init and every shim are compiled with the fixed
crypto/x509 from now on. An app's own TLS stack only gets the fix when the
app is compiled with ≥1.27.3 too; any app importing a gosd public package
now gets that floor through go.mod's MVS.

**go-diskfs v1.9.4 (bean gosd-qvjs warning).** Not chosen — tailscale.com
v1.104.2 requires it. Diffed v1.9.3 → v1.9.4: `filesystem/fat32` and
`partition/mbr` non-test sources are byte-identical (the only fat32 change is
a test de-flake), so fat32limit.go / fat32selfconsistent.go and
blockmount's label round-trip model still describe the code they model. The
remaining changes (BLKRRPART → BLKPG fallback in `ReReadPartitionTable`, an
`OpenFromPathWithExclusive` backend, ext4/iso9660/squashfs/gpt work) don't
touch anything gosd calls on a block device: `internal/image` only
partitions image files, where `ReReadPartitionTable` is a no-op. The go.mod
comment is left in place since it still applies to any future bump.

**pion/mdns v2.1.0 → v2.2.2.** `mdns.Server(…, *Config)` is deprecated in
favour of `NewServer(…, ...ServerOption)`. Migrated with the exact options
the legacy shim applied (`WithRecordTypes(A, AAAA)`,
`WithCacheRefresh(false)`, plus name/local names), so responder behaviour is
unchanged. NewServer still doesn't close the handed-in sockets on its error
paths, so gosd-o6tp's close-on-failure stays.

**nix.** nixos-unstable has Go 1.27.3 only as `go_1_27`/`go_latest`; its
unversioned `go` is still 1.26.9. The flake now uses
`buildGoLatestModule`/`go_latest` for both the build and the run-time PATH
fallback, so a lagging default `pkgs.go` can't strand the nix build again.
Built locally against the new nixpkgs rev (02c087f) and `gosd version`
reports go1.27.3. The lock's narHash was computed from a `git archive` of
that rev because the session couldn't fetch GitHub tarballs; CI's nix build
job is the check that it matches.

**Lint.** golangci-lint's prebuilt release binaries are built with whatever
Go was current at release and refuse a newer go.mod directive outright, so
CI now uses `install-mode: goinstall` (built by the setup-go toolchain) at
v2.15.0.

**Not done: Mozilla CA bundle (`internal/cacerts`).** curl.se was
unreachable from this session, so the dated snapshot pin wasn't bumped. The
x509 fix concerns intermediates, which aren't in the roots bundle, so this
PR doesn't depend on it — but a fresh pin is a sensible follow-up.
