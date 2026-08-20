---
# gosd-mf3a
title: gosd build has no surface for extra config.txt lines or kernel cmdline params
status: todo
type: task
priority: normal
created_at: 2026-07-29T21:45:25Z
updated_at: 2026-08-20T04:58:21Z
parent: gosd-qkbl
---

Found while doing [[gosd-qkbl]]: HDMI audio needs `snd_bcm2835.enable_hdmi=1`
on the kernel cmdline, and an app has no way to add it. There is currently no
surface at all for extra kernel cmdline params or extra `config.txt` lines.

## DECISION (JP, 2026-08-20): a board-agnostic `--kernel-param`

`gosd build --kernel-param key=value`, repeatable. GoSD bakes each param into
whatever the selected board's family actually uses — `cmdline.txt` for the Pi
family, U-Boot `bootargs` for the mainline fleet — so the developer writes the
parameter once and it works across boards.

**No raw `config.txt` escape hatch.** A `--config-txt-line` flag would put a
Pi-family-specific surface on a board-agnostic command and would quietly
invite apps that only build for Pi, which cuts against a bare `gosd build`
producing every public board's image. If a Pi firmware setting is ever needed
that genuinely has no cmdline equivalent (`dtoverlay`, `gpu_mem`), that gets
its own typed, board-aware flag and its own bean — not a raw passthrough.

## Locked decisions

- **This is developer build input and on-card ABI**, in the same category as
  `--boot-size` and `--data-filesystem` — NOT an operator setting. It
  therefore belongs on `gosd build`, and explicitly **not** in the config tree
  ([[gosd-rw6n]]). The config tree is what the person holding the card edits;
  this is what the developer compiles in. Do not add a `config/` key for it,
  and do not add a `GOSD_*` env override.
- Mirror it onto `gosd run` if and only if `gosd run` already mirrors
  `--boot-size`/`--data-filesystem`; match whatever that precedent is rather
  than inventing a different one.
- **Params are per-family-delivered but board-agnostic to write.** The
  developer never says "put this in cmdline.txt". If a param is meaningless on
  a selected board, that is fine and silent — `snd_bcm2835.enable_hdmi=1` on a
  Rockchip board is inert, exactly as an unknown cmdline param normally is.
- **Validate the shape, not the vocabulary.** Reject params that would break
  the cmdline (embedded whitespace, newlines, NULs, a missing `=` where the
  form requires one) with an actionable error naming the offending value.
  Do NOT maintain an allowlist of known-good kernel parameters — it would go
  stale and block legitimate use.
- **Deterministic ordering.** Params must land in a stable order so two builds
  of the same app produce byte-identical boot files; the repo's determinism
  conventions are mechanically checked ([[gosd-8pgg]]).
- **Pure Go, no artifacts release.** This is CLI/image-assembly work only —
  it changes what gosd writes onto the boot partition, not any compiled
  artifact, so it needs no kernel rebuild and no `artifacts/vX.Y.Z` tag.

## Todo

- [x] JP decision: board-agnostic `--kernel-param`, no raw config.txt passthrough
- [ ] `--kernel-param` on `gosd build` (repeatable), plumbed through the pipeline
- [ ] Per-family delivery: Pi `cmdline.txt`, mainline-fleet U-Boot `bootargs`
- [ ] Shape validation with actionable errors; stable ordering
- [ ] Mirror onto `gosd run` if that matches the `--boot-size` precedent
- [ ] Fixture-driven integration test: read the built image back and assert the
      param landed in the right file for a Pi board AND a fleet board, with the
      network-tripwire RoundTripper proving no fetch happened
- [ ] Document it, and unblock [[gosd-qkbl]]'s HDMI audio case as the worked example
- [ ] Change file (user-facing new flag)
