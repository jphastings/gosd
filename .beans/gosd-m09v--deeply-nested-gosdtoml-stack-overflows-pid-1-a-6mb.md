---
# gosd-m09v
title: 'Deeply-nested gosd.toml stack-overflows PID 1: a ~6MB file on the card bricks the board permanently'
status: todo
type: bug
created_at: 2026-08-12T04:13:13Z
updated_at: 2026-08-12T04:13:13Z
---

**Severity: Critical.** Unrecoverable, permanent, and reachable by anyone who
can write one file to a FAT partition. The device cannot self-heal; the owner
must pull the card and edit it on another computer.

## Verified

`internal/gosdtoml/config.go:176` calls
`toml.NewDecoder(...).Decode(&raw)`. `BurntSushi/toml` v1.6.0's parser is
recursive descent with **no depth limit**. Measured against this repo's own
`gosdtoml.Parse`, on this repo's pinned dependency:

| nesting depth | file size | result |
|---|---|---|
| 200,000 | 0.38 MiB | parses, `err=<nil>` |
| 1,000,000 | 1.91 MiB | parses, `err=<nil>` |
| 3,000,000 | 5.72 MiB | `runtime: goroutine stack exceeds 1000000000-byte limit` / `fatal error: stack overflow` |
| 8,000,000 | 15.26 MiB | same |

The test wrapped the call in `defer recover()`. **It never fired** — a Go
stack-overflow is a runtime fatal error, not a recoverable `panic`. So
`boot.PanicGuard`'s `recover()` (cmd/gosd-init/internal/boot/panicguard.go)
cannot catch it either, and neither can anything else.

Note the 1 GiB figure is Go's default max stack. A board with 512 MB RAM
runs out of physical memory first, so **real hardware fails sooner and at a
smaller file than this test suggests**.

## Attack

1. Attacker writes `gosd.toml` to the boot partition — plug the card into
   any computer, it is a plain FAT32 volume. The file is one line:
   `a = ` + 3,000,000 `[` + `1` + 3,000,000 `]` (~6 MB; the partition
   defaults to 256 MiB, so size is no obstacle).
2. Every boot: `boot.Run` -> `readGosdToml` (cmd/gosd-init/main.go:320-329)
   -> `gosdtoml.Parse` -> stack overflow -> **PID 1 dies**.
3. Linux panics ("Attempted to kill init!"). The file persists, so this
   repeats forever. A deterministic, permanent boot loop.

The device's only channel back to a human — `LAST_FATAL_ERROR.md` — is never
written, because the crash happens before any report can be produced and is
not catchable. The owner sees a board that does nothing.

## Second route in

The same `gosdtoml.Parse` is called on snapshot content at
`cmd/gosd-init/internal/provsnapshot/provsnapshot.go:680`, after only an
**unkeyed** SHA-256 check (`:676`) that an attacker computes themselves.
So a bomb planted in `/data` reaches the same crash — and `/data` survives a
reflash and cannot be cleared from a macOS host when it is ext4. See the
sibling bean on snapshot authenticity.

## Fix

Cap the size before parsing, in all three unbounded readers. None of them
bounds its read today:

- `cmd/gosd-init/main.go:320-329` (`readGosdToml`)
- `cmd/gosd-init/main.go:285-291` (`readConfig`)
- `internal/provision/provision.go:123-132` (`readOptional`, cloud-init)

Use the `Stat`-then-check pattern this codebase already uses at
`cmd/gosd-init/internal/boot/platform_linux.go:194-195` for
`secretreg.Path`. A 64 KiB cap is orders of magnitude above any real
gosd.toml (`gosdtoml.Render`'s own output is well under 4 KiB) and defeats
the bomb outright, since the crash needs megabytes of nesting syntax.

A size cap is the practical fix: Go offers no per-goroutine stack cap
(`debug.SetMaxStack` is process-global and would also cap legitimate deep
call chains).

## Todos

- [ ] Size-cap `readGosdToml` before `Parse`
- [ ] Size-cap `readConfig` before JSON decode
- [ ] Size-cap `provision.readOptional` for user-data and network-config
- [ ] Size-cap the snapshot's gosd.toml read in provsnapshot
- [ ] Test: an oversized card file is rejected with a logged, actionable message and boot continues on baked defaults
- [ ] Test: a nesting bomb at the cap boundary parses or rejects, never crashes
