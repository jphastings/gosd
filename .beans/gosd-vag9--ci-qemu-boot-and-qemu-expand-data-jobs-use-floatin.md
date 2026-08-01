---
# gosd-vag9
title: 'CI: qemu-boot and qemu-expand-data jobs use floating action tags instead of the repo''s pinned-SHA discipline'
status: todo
type: task
priority: low
created_at: 2026-07-31T07:54:33Z
updated_at: 2026-07-31T07:54:33Z
---

Found by review sweep `gosd-fuxs` (kernel/CI infra area), verified.

ci.yml lines 164-165, 177, 241-242, 247: actions/checkout@v7,
actions/setup-go@v6, actions/cache@v4 — every other uses: line in both
workflows pins a full 40-char SHA with a version comment. These two jobs
run with network access and sudo; a retagged upstream ref changes their
behavior with no diff in our history — the supply-chain exposure the
SHA-pinning convention exists to close.

**Fix:** pin all six lines to the same SHAs used elsewhere in the file.
