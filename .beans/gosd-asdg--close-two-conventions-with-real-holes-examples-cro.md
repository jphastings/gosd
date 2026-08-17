---
# gosd-asdg
title: 'Close two conventions with real holes: examples cross-compile coverage and the netlink Request flag'
status: completed
type: bug
priority: normal
created_at: 2026-08-17T16:57:20Z
updated_at: 2026-08-17T21:08:20Z
parent: gosd-8pgg
---

Part of epic gosd-8pgg. Stacked on gosd-ihdn (needs `internal/repocheck`).
Independent of the other repocheck children — different files, no conflict.

Two CLAUDE.md conventions that are stated as absolute and are **not actually
true of the tree today**. These are real defects the audit surfaced, not lint
findings.

## Part 1 — examples cross-compile coverage

CLAUDE.md: "Examples ... must cross-compile for every board arch (arm64 AND
`GOARCH=arm GOARM=6`)". ci.yml's `smoke-build` job hand-maintains the package
list twice. `examples/` has 10 directories; the arm64 line lists 9 (no
`sattrack`) and the armv6 line lists 8 (no `sattrack`, no `usbserial`).

### First step decides the shape — do this before writing anything

Run both builds:

    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./examples/...
    CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build ./examples/...

`go list -deps` resolves cleanly for every example under both settings, so
nothing is excluded by build constraints — but `sattrack`'s DRM dependency may
carry 64-bit assumptions that only surface at compile time, which is the
likeliest reason it was dropped from *both* lines.

- **Both pass** -> replace the hand-maintained lists with `./examples/...`,
  deleting the drift class outright. Then `internal/repocheck/examplesci_test.go`
  is ~15 lines: assert both `run:` strings still contain `./examples/...` and
  no per-example token, so nobody quietly re-expands the list.
- **Something genuinely fails on armv6** -> keep the explicit list for that leg
  and write the full parity test: parse ci.yml with `gopkg.in/yaml.v3` (already
  a direct dependency; `internal/kernelspec/workflow_test.go` is the precedent),
  locate job `smoke-build`, match the two steps by their `name:` fields,
  tokenise each `run:` and keep `./examples/`-prefixed tokens, diff against
  `os.ReadDir("../../examples")` **in both directions** (a stale token after a
  rename must fail too). Add
  `exemptFromCrossCompile map[string]map[string]string` (example -> arch ->
  reason) with the **reason asserted non-empty**, and amend CLAUDE.md to say an
  example absent from a CI line needs an entry there.
  Also make ci.yml:135-141's rule mechanical: if `./examples/chime` is absent
  from the armv6 line, `./sound` must be present — that comment explains the
  `sound` package's ioctl request numbers embed struct sizes that differ
  32-vs-64-bit, and it is the reason chime is in that list at all.

**Either way `examples/sattrack` belongs on the arm64 line** — there is no
plausible reason for its absence there, and CLAUDE.md names it as the reference
"bigger example".

## Part 2 — the netlink Request flag

CLAUDE.md: raw `mdlayher/netlink` calls MUST OR `netlink.Request` into their
Execute/Send flags — the kernel silently *skips* a non-Request message while
still returning a success ack, a no-op dressed as success. Two bench days went
to this (gosd-anyp). CLAUDE.md names
`cmd/gosd-init/internal/wifiup/connect_linux_test.go` as the pattern to "mirror
for any new raw-netlink call".

There are two flag consts in `wifiup/platform_linux.go`:
`connectRequestFlags` (line ~108) and `wiphyDumpFlags` (line ~188). **Only the
first is pinned by a test.** Mirror the existing test for the second.

Keep it behavioural in the spirit CLAUDE.md means: the point is that the wire
flags carry `netlink.Request`, not that a constant has a particular numeric
value.

## Todo

- [x] Run both cross-compile commands and record the result here — it picks the branch below

**Result (2026-08-17): BOTH PASS.** `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./examples/...` and `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build ./examples/...` both exit 0 over all 10 example directories, and `go list` resolves all 10 under both settings (nothing excluded by build constraints). `examples/sattrack`'s DRM dependency carries no 32-bit obstacle — it builds for armv6 too. So the wildcard branch applies: no exemption map is needed, and the sattrack/usbserial omissions were pure list drift, not a real constraint.
- [x] Either switch ci.yml to `./examples/...`, or fix the lists and add the exemption map
- [x] `internal/repocheck/examplesci_test.go` in whichever shape the result dictates
- [x] Add the missing `wiphyDumpFlags` test alongside `connect_linux_test.go`'s
- [x] Amend CLAUDE.md's examples bullet to point at the enforcement (no exemption map: the wildcard branch applied)
- [x] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

If `sattrack` genuinely cannot build for armv6, that is worth a follow-up bean
of its own rather than being quietly absorbed into an exemption — say so.

No changeset — internal only, use the `no release notes` label.


## Summary of Changes

Both cross-compile commands pass over all ten examples, so Part 1 took the
wildcard branch: no example is genuinely unbuildable, and no exemption map
was needed.

- `.github/workflows/ci.yml` — `smoke-build`'s two hand-maintained package
  lists collapse to `./examples/...` on both legs. That closes the three
  example/arch pairs CI never compiled (`sattrack` on both arches,
  `usbserial` on armv6) and removes the drift class: a new example is
  covered the moment its directory exists. The armv6 leg now also names
  `./sound` outright rather than reaching it through `examples/chime`'s
  dependency list — armv6 is the only place the ALSA ioctl struct-size
  assertions get checked, and that coverage should not be a side effect of
  one example's imports. The old "don't drop chime without adding ./sound"
  comment is therefore gone, having become unconditional.
- `internal/repocheck/examplesci_test.go` (new, `package repocheck`) —
  parses ci.yml with `gopkg.in/yaml.v3`, locates the `smoke-build` job, and
  for each of the two board architectures asserts its step builds
  `./examples/...` and names no example individually. Parsing YAML rather
  than grepping keeps the job's own comments (which mention example paths)
  out of what is matched. Negative-checked by re-expanding the armv6 leg
  into `./examples/hello ./examples/chime`, which fails the test on both
  counts.
- `cmd/gosd-init/internal/wifiup/connect_linux_test.go` —
  `TestWiphyDumpFlagsIncludeRequest` mirrors the existing
  `connectRequestFlags` test for `wiphyDumpFlags`, the package's other raw
  netlink call. Bit-tests `netlink.Request` (without it the GET_WIPHY dump
  is skipped, comes back empty, and every phy reads as lacking
  `4WAY_HANDSHAKE_STA_PSK` — so `ConnectPSK` is refused on hardware that
  supports it) and `netlink.Dump` (a split wiphy dump would otherwise be
  truncated to its first message).
- `CLAUDE.md` — the examples bullet now names the enforcement, says there is
  deliberately no exemption mechanism, and records why `./sound` is on the
  armv6 leg.

Internal only; `no release notes` label, no changeset.

### Findings worth keeping

- `examples/sattrack` builds for **both** arches, armv6 included. Its
  absence from both CI legs was list drift, not a 64-bit constraint in the
  DRM dependency — so no follow-up bean is needed on that front.
- `go list ./examples/...` resolves all ten examples under both settings, so
  the wildcard is not silently skipping anything today. The residual risk it
  carries differs from the old one: `./examples/...` would *silently skip* an
  example whose files became build-constrained away for an arch, where an
  explicit list would have errored. Worth knowing if an example ever grows a
  `//go:build` guard.
