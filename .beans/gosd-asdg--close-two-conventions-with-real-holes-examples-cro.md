---
# gosd-asdg
title: 'Close two conventions with real holes: examples cross-compile coverage and the netlink Request flag'
status: todo
type: bug
priority: normal
created_at: 2026-08-17T16:57:20Z
updated_at: 2026-08-17T16:57:48Z
parent: gosd-8pgg
blocked_by:
    - gosd-ihdn
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

- [ ] Run both cross-compile commands and record the result here — it picks the branch below
- [ ] Either switch ci.yml to `./examples/...`, or fix the lists and add the exemption map
- [ ] `internal/repocheck/examplesci_test.go` in whichever shape the result dictates
- [ ] Add the missing `wiphyDumpFlags` test alongside `connect_linux_test.go`'s
- [ ] Amend CLAUDE.md's examples bullet to point at the enforcement (and document the exemption map, if that branch was taken)
- [ ] Quality gates (go test / go vet / gofmt / golangci-lint x2)

## Notes

If `sattrack` genuinely cannot build for armv6, that is worth a follow-up bean
of its own rather than being quietly absorbed into an exemption — say so.

No changeset — internal only, use the `no release notes` label.
