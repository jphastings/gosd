---
# gosd-mf3a
title: gosd build has no surface for extra config.txt lines or kernel cmdline params
status: completed
type: task
priority: normal
created_at: 2026-07-29T21:45:25Z
updated_at: 2026-08-21T08:01:11Z
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
- [x] `--kernel-param` on `gosd build` (repeatable), plumbed through the pipeline
- [x] Per-family delivery: Pi `cmdline.txt`, mainline-fleet U-Boot `bootargs`
- [x] Shape validation with actionable errors; stable ordering
- [x] Mirror onto `gosd run` if that matches the `--boot-size` precedent
- [x] Fixture-driven integration test: read the built image back and assert the
      param landed in the right file for a Pi board AND a fleet board, with the
      network-tripwire RoundTripper proving no fetch happened
- [x] Document it, and unblock [[gosd-qkbl]]'s HDMI audio case as the worked example
- [x] Change file (user-facing new flag)

## Summary of Changes

`gosd build --kernel-param key=value` (repeatable) ships, mirrored onto
`gosd run`, exactly as decided above: shape validation only, deterministic
ordering, no config-tree key and no `GOSD_*` override, no raw `config.txt`
passthrough, and no artifacts release.

**Per-family delivery.** `boards.BuildConfig` gained `KernelParams []string`
plus a `KernelParamString()` helper, so the joining rule lives in one place.
Every board's boot-config template appends `{{with .KernelParams}} {{.}}{{end}}`
to the kernel command line it already renders — `cmdline.txt` on the three Pi
boards, `extlinux.conf`'s `append` line (which U-Boot turns into `bootargs`)
on radxa-zero-3e, nanopi-zero2, rock-4se and cubie-a5e. Nothing else about
those templates changed, so an image built without the flag renders the exact
bytes it rendered before, with no trailing space.

**qemu-virt is the one board with nowhere in the image to put them**, since
qemu is handed `-kernel`/`-initrd`/`-append` directly and the profile writes
no boot config at all. `gosd run --kernel-param` therefore extends
`qemurun.Args`' `-append` instead (`qemurun.Options.KernelParams`), which is
that board's real kernel command line; both the qemuvirt package doc and
`gosd run`'s flag help say so. An image built with
`gosd build --board=qemu-virt --kernel-param ...` carries no record of it —
the honest consequence of qemu-virt having no boot config, not a silent
failure elsewhere.

**Mirrored onto `gosd run`** because that matches the precedent, not despite
it: `gosd run` mirrors `--boot-size` ("useful for checking a large app still
fits before a real build") while declining `--data-filesystem` as a
shipping-image concern the qemu smoke test doesn't have. A kernel parameter is
squarely the first kind — trying one under qemu beats a reflash to find out —
and, unlike `--data-filesystem`, it is genuinely deliverable on qemu-virt.

**Validation** is `internal/kernelparam`, a new package whose doc comment
states the shape-not-vocabulary rule so a future reader doesn't reintroduce an
allow-list. It refuses an empty value, embedded whitespace or tabs, newlines
and carriage returns, NULs, other control bytes, and a leading `=` (no
parameter name), each with an error naming the flag and quoting the offending
value. Everything else passes through untouched, including parameters gosd has
never heard of — the test asserts `totally.made.up=yes` builds fine, because
that is the property the locked decision is about.

**Ordering** is the order the developer wrote, not sorted: either is
reproducible, but only this way does the kernel's last-one-wins handling of a
repeated parameter stay under the developer's control.

**Tests.** `internal/kernelparam` gets the shape table.
`TestBuildKernelParamsReachEachFamilysBootConfig` builds pi-zero-2w and
radxa-zero-3e from the fake-artifacts fixtures behind the network tripwire,
reads each image's boot partition back, and byte-compares the whole
`cmdline.txt` / `extlinux.conf` — one of the two parameters is
`snd_bcm2835.enable_hdmi=1`, so the Rockchip case also pins "meaningless is
still delivered, inert". `TestBuildWithoutKernelParamsLeavesBootConfigUnchanged`
pins the additive-only contract, and there are refusal tests on both `gosd
build` and `gosd run` plus a `qemurun.Args` test for the `-append` extension.

**Docs.** A new section in the runtime contract covers per-family delivery,
inertness, shape-only validation, ordering, why it is developer input rather
than a device setting, the `gosd run` mirror, and hand-editing the file on an
already-flashed card. The audio guide now names
`--kernel-param snd_bcm2835.enable_hdmi=1` as the worked example, alongside
the `enable_hdmi` patch its recipe ships (see [[gosd-qkbl]]).

No COMPATIBILITY.md change: the flag behaves identically on every public
board, so there is no per-board variance to record — the same reason
`--console-baud` has no row.
