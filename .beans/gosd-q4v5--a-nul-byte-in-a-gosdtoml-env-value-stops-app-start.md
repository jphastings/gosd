---
# gosd-q4v5
title: A NUL byte in a config/ setting stops /app starting, forever, on every boot
status: completed
type: bug
priority: normal
created_at: 2026-08-12T04:18:42Z
updated_at: 2026-08-20T05:36:40Z
---

**Re-scoped 2026-08-20.** Filed against `internal/gosdtoml`'s `coerceEnv`,
which no longer exists: the per-attribute config tree replaced `gosd.toml`
wholesale (epic `gosd-rw6n`). The TOML backslash-u escape that carried the
NUL is gone; the defect is not. An app's environment now comes from files in
`config/env/` on the boot partition, where the **file's name** is the
variable's name and its contents are the value — and neither was checked at
all.

## Verified, against the current code

`cardconfig.Read` -> `Tree.Group("env")` -> `boot.mergeUserEnv` ->
`deps.AppStarter.Start(opts.AppPath, env, ...)`. Measured on this repo's Go:

    cmd.Env = []string{"FOO=bar" + NUL + "baz"}; cmd.Start()
    -> exec: environment variable contains NUL

`os/exec` rejects the **whole environment**, not the offending entry, so
/app never starts. What the device does then (`boot.Supervisor.runOnce`) is
log `starting /app failed: ...` and retry with backoff, forever: a start
failure never reaches `OnExit`, so no crash report is written, the status
LED never leaves "booting", and nothing halts. The board looks alive and
does nothing, on this boot and every later one, because the file is still on
the card.

Reproduction: write a single NUL byte into `config/env/ANY_NAME` on the FAT
boot partition. No tools beyond a hex editor and the card.

## A second, quieter hole in the same place

The build validates env names (`configtree.checkEnvValue`, shape
`^[A-Za-z_][A-Za-z0-9_]*$`) — but nothing did at runtime, and a file created
on the card never went through the build. `=` and space are both legal FAT
long-file-name characters, so `config/env/FOO=BAR` holding `secret` reached
the app as `FOO` holding `BAR=secret`: silently a different variable than
the file's name says. A name like `9LIVES` or `A B` reached it as something
no shell could name back.

Not a brick like the NUL, but the same root cause — the runtime trusted a
name and a value the build would have refused — so it is fixed in the same
pass.

## Fix

Both gates, as specified below, are **on `main` already** — reached
independently from the `/data` side by `gosd-7m9y` and `gosd-39da` (PR
#345) while this bean sat in review, because the settings restored from the
unauthenticated data partition need the identical checks the card's own
files do. This bean's branch carries neither; they are recorded here as the
resolution rather than re-implemented beside them.

- `cardconfig.readValue` refuses a setting file containing a NUL byte, logs
  which file, and leaves the baked value standing. It sits there rather
  than in the env path because a setting is text somebody typed whatever it
  configures, and one gate is easier to keep true than five. On `main` the
  test itself is `configtree.PlausibleValue`, shared with `configstore`.
- `boot.mergeUserEnv` drops a name that isn't a valid environment variable
  name, beside the existing `GOSD_*` reservation check and logging the same
  way. The shape is `configtree.ValidEnvName`, exported so the build and the
  device can't drift on what a name is — the same reason
  `configtree.TrimValue` is exported.

## Todos

- [x] Establish how card values reach the app's environment now
- [x] Confirm the NUL failure mode against the current code and Go's exec
- [x] Refuse a setting file holding a NUL, falling back to the baked value
- [x] Refuse a card env name the build would have refused
- [x] Export one env-name shape so build and runtime can't drift
- [x] Test: a NUL in a setting doesn't stop /app, and the rest of the tree still reads
- [x] Test: `config/env/FOO=BAR`, `A B` and `9LIVES` are dropped with a log line each, and /app starts

## Summary of Changes

Re-scoped from `gosdtoml.coerceEnv` to the config tree, then verified
against the current code: the NUL brick and the unchecked env name are both
real on the surface that replaced it.

Fixed at two gates: `cardconfig.readValue` skips a setting whose bytes
aren't text, and `mergeUserEnv` drops a card env name that isn't a valid
environment variable name, using `configtree.ValidEnvName` — the same
regexp the build has always enforced, exported so there is one answer
rather than two. Both log an actionable line naming the file on the card and
fall back to the value the image was built with, so the failure mode is a
setting that didn't take effect rather than a device that doesn't run.

**Both gates were shipped on `main` by `gosd-7m9y`/`gosd-39da` (PR #345),
not by this bean's branch**, which was rebased onto them and dropped its own
copies as redundant. The `/data` restore path and the card's own files reach
the same two functions, so one pair of checks covers both routes — which is
the right outcome, and the reason nothing was re-implemented alongside.
