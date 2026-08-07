---
# gosd-vag9
title: 'CI: qemu-boot and qemu-expand-data jobs use floating action tags instead of the repo''s pinned-SHA discipline'
status: completed
type: task
priority: low
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-08-07T17:00:56Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area), verified.

ci.yml lines 164-165, 177, 241-242, 247: actions/checkout@v7,
actions/setup-go@v6, actions/cache@v4 — every other uses: line in both
workflows pins a full 40-char SHA with a version comment. These two jobs
run with network access and sudo; a retagged upstream ref changes their
behavior with no diff in our history — the supply-chain exposure the
SHA-pinning convention exists to close.

**Fix:** pin all six lines to the same SHAs used elsewhere in the file.


---

## Todos

- [x] Audit every workflow under `.github/workflows/` for floating action refs (not just the two named jobs)
- [x] Resolve each floating tag's exact current commit via `gh api .../git/ref/tags/<tag>`
- [x] Pin `qemu-boot`'s `actions/checkout`, `actions/setup-go`, `actions/cache` to full SHAs with trailing version comments
- [x] Pin `qemu-expand-data`'s same three actions the same way
- [x] Validate with `actionlint`
- [x] Run full quality gates (`go test`, `go vet`, `gofmt`, `golangci-lint` host + linux)

## Summary of Changes

Audited all three workflow files (`ci.yml`, `build-artifacts.yml`, `publish-npm.yml` -
`cacerts-pin-check.yml` from unmerged PR #197 does not exist on `main` and was not
touched). The only floating refs found were exactly the six lines this bean names,
all in `ci.yml`'s `qemu-boot` and `qemu-expand-data` jobs:

- `actions/checkout@v7` -> `3d3c42e5aac5ba805825da76410c181273ba90b1` (`v7.0.1`,
  what the `v7` tag currently resolves to - one patch ahead of the `v7.0.0` pin
  used elsewhere in the file; verified via the GitHub compare API that v7.0.1 is
  a small, non-breaking patch release over v7.0.0)
- `actions/setup-go@v6` -> `924ae3a1cded613372ab5595356fb5720e22ba16` (`v6.5.0`,
  matches the pin already used everywhere else in the file)
- `actions/cache@v4` -> `0057852bfaa89a56745cba8c7296529d2fc39830` (`v4.3.0`,
  matches the pin already used everywhere else in the file)

Each SHA was resolved live via `gh api repos/<owner>/<repo>/git/ref/tags/<tag>`,
not copied blind from elsewhere in the file - worth recording since `setup-go`
and `cache`'s floating tags happened to already match the file's existing pins,
but `checkout`'s `v7` tag has moved on to a new patch release since this bean
was filed, so pinning it to "the exact version currently referenced" (per the
supply-chain intent this bean exists to close) lands one patch ahead of the
`v7.0.0` used elsewhere. No action's version was upgraded relative to what its
own floating tag already resolved to.

Also refreshed a stale comment on the `qemu-disk-ext4` job that described this
cleanup as a "future fix" still pending.

Verified with `actionlint` (the one pre-existing finding, a shellcheck SC2129
style nit in `publish-npm.yml`, predates this change and is unrelated).
