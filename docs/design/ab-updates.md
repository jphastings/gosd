# Design spike: over-the-network app updates (app-slot scheme)

Status: design only, nothing in this document is built. Tracked by bean
`gosd-v2w1`. Target: informing a v0.4 task breakdown.

**Revision note:** this document originally recommended a per-board
boot-slot mechanism (Pi `tryboot`, Radxa U-Boot `bootcount`). JP rejected
that recommendation: GoSD intends to support many boards over time, and a
distinct bootloader-level A/B mechanism per board does not scale as a
maintenance burden. This revision replaces it with a single board-agnostic
**app-slot** scheme. The rejected research is preserved in Appendix A, both
for its citations and because the reasoning in it (FAT's weak atomicity,
gokrazy's two-phase commit pattern) still informs the accepted design below.

## 0. Scope decision: OTA updates replace the app only

**The kernel, initramfs, and bootloader are reflash-only. They are never
touched by an over-the-network update.** This is a deliberate scope cut, not
an oversight:

- The initramfs-baked `/app` — the **FACTORY** image — is built once by
  `gosd build` and never modified in place. It is always present and always
  known-good, because nothing in the update path ever writes to it.
- Everything an OTA update changes lives inside the existing single
  `GOSD-BOOT` FAT32 partition (`internal/image/image.go`'s locked layout:
  one 256MiB partition, no MBR/partition-table change of any kind), as
  ordinary files alongside the kernel/initramfs/board-config the pipeline
  already writes there.
- If the kernel, initramfs, or a board's bootloader needs to change, that
  requires a full `gosd build` + reflash. There is no in-place mechanism for
  it, and this document does not propose one (see Appendix B for the escape
  hatch if that ever changes).

This scope cut is what makes a single, board-agnostic scheme possible: the
previous recommendation needed per-board mechanisms specifically because the
kernel+initramfs pair was the unit of update, and Pi/Radxa boot in
fundamentally different ways. An app binary sitting inside an already-booted
Linux userspace has no such asymmetry — the same file layout, commit
protocol, and supervisor logic work identically on every current and future
board.

## 1. Why board-agnostic, in one paragraph

Pi Zero 2 W and Radxa Zero 3E already need two different bootloader-level
mechanisms (`tryboot` vs. U-Boot `bootcount`) because they boot in genuinely
different ways, and CLAUDE.md's board list is expected to grow. Building and
maintaining a bespoke A/B mechanism per board — verifying firmware behavior
on real hardware, keeping U-Boot config in sync, handling each board's own
failure modes — is a cost that scales linearly with the number of supported
boards, forever. An app-slot scheme scoped to userspace has a fixed cost:
write it once against Linux syscalls and a FAT partition, and every future
board gets it for free. Full research and citations for the rejected
per-board approach are kept in Appendix A.

## 2. The FACTORY image and the two mutable slots

Three things can be running as `/app` at any given boot, forming the ladder
§5 falls down:

1. **FACTORY** — the initramfs-baked `/app`, exactly as `gosd build`
   produced it (`internal/pipeline.Assemble` writes it to
   `initramfs.File{Path: "/app", ...}` today, unchanged by this design).
   Always present, never updated, the floor everything else can fall back
   to.
2. **Slot A / Slot B** — two app binaries living as ordinary files in the
   `GOSD-BOOT` partition. An update always writes to whichever slot is
   *not* currently active, mirroring gokrazy's "never write to the running
   thing" discipline (already identified as transferring directly, in the
   original research below).

Exactly one of FACTORY/A/B is "current" at any boot, recorded in a small
state file (`slot.state`, §3). A freshly flashed image has no `app.a`/
`app.b` files at all — `slot.state` is absent, and gosd-init's fail-safe
parsing rule (§4) treats that identically to "run FACTORY."

## 3. File layout on GOSD-BOOT

No partition-table change. A new directory inside the existing FAT32
`GOSD-BOOT` partition:

```
/boot/app/
  app.a           - slot A binary (absent until first pushed)
  app.b           - slot B binary (absent until first pushed)
  app.a.tmp       - transient, exists only mid-push (write target before rename)
  app.b.tmp       - transient, exists only mid-push
  slot.state      - which slot boots next, and whether it's on probation
  slot.state.tmp  - transient, exists only mid-activate
```

`slot.state` is a small fixed-format file (JSON is fine — it's at most a few
hundred bytes, no need to hand-roll a binary format) with these fields:

```go
type SlotState struct {
    Target     string // "factory" | "a" | "b" — what to boot next
    Status     string // "committed" | "probation"
    Previous   string // the last-known-good target, for the ladder's rung 2
    Generation uint64 // monotonic, incremented on every write
    SHA256     string // hex digest of Target's binary, "" for factory
    Size       int64  // byte size of Target's binary, 0 for factory
}
```

`SHA256`/`Size` let gosd-init cheaply re-verify the slot binary at every
boot (not just at push time) — worthwhile because FAT has no built-in
corruption detection, and a slot file can in principle degrade quietly
between the boot that committed it and a much later boot that finally
re-reads it.

## 4. Commit protocol

Every write to `GOSD-BOOT` follows the same shape: write to a temp name,
`fsync` the data, `rename` over the real name. This is the same
write-then-rename discipline the rejected design used for `autoboot.txt`/
U-Boot env — reused here because the underlying medium (FAT32, no journal,
directory-entry rename is not power-loss-atomic) hasn't changed. What has
changed is that there are now two independent commits in sequence — the app
binary, then the state file — and the design has to be explicit about the
gap between them.

**Push phase** (server-side handling of `PUT /update`, see §6):
1. Stream the request body to `/boot/app/app.<inactive>.tmp`, verifying the
   HMAC as bytes arrive (§6.3) — never buffer the whole binary in RAM given
   both boards are memory-constrained (`docs/runtime.md`'s "everything is
   RAM-resident" caveat applies to gosd-init itself, not just `/app`).
2. On a full, HMAC-verified body: `fsync` the temp file's data, then
   `rename` it to `/boot/app/app.<inactive>`. On any verification failure,
   delete the temp file and return an error — nothing renamed, nothing
   referenced.

**Activate phase** (server-side handling of `POST /update/activate`):
1. Build the new `SlotState{Target: <inactive>, Status: "probation",
   Previous: <current Target>, Generation: current+1, SHA256, Size}`.
2. Write it to `slot.state.tmp`, `fsync`, `rename` to `slot.state`.
3. Reboot (`deps.Rebooter.Reboot()` — already wired for the fatal path in
   `cmd/gosd-init/internal/boot/sequence.go`; this is a new caller of an
   existing capability, not new machinery).

### What survives power loss, at each step

| Crash point | On-disk state after crash | Next boot does |
|---|---|---|
| Before any write | Unchanged | Boots current `Target` exactly as before — no observable change |
| Mid-write of `app.<slot>.tmp` | `.tmp` partial/garbage; live `app.<slot>` (if any) and `slot.state` untouched | Boots current `Target`, unaffected — nothing references the `.tmp` file |
| `.tmp` fsynced, crash mid-rename to `app.<slot>` | Directory entry for `app.<slot>` may be torn (FAT rename is a metadata op, not power-loss-atomic) or simply not yet updated | Boots current `Target`, unaffected — `slot.state` still names the old target, so the (possibly corrupt) inactive slot file is never read |
| `app.<slot>` renamed and durable, crash before `slot.state.tmp` write | New binary sits on disk, inert | Boots current `Target`, unaffected; a retried push simply overwrites `app.<slot>.tmp`/`app.<slot>` again |
| `slot.state.tmp` fsynced, crash mid-rename to `slot.state` | `slot.state` may be torn/unparseable | **Fail-safe rule** (below): treat as `Target=factory, Status=committed` |
| `slot.state` renamed and durable | New `Target`/`Status` is the committed fact | Boots the new slot under probation (§5) — this rename is the one true commit point of the whole operation |

**Fail-safe parsing rule, stated explicitly:** if `slot.state` is missing,
unparseable, or its `Generation`/format looks inconsistent, gosd-init always
resolves to `Target=factory, Status=committed` — never to `a` or `b`. This
is the same principle the rejected design already argued for ("detect the
commit file looks garbled and fail toward the last-known-good... not an
undefined one") — applied here with a stronger guarantee than the rejected
design could offer, because FACTORY is *always* known-good by construction
(never written by an update), whereas the rejected scheme's fallback
(`config.txt`/`[all]`, U-Boot's `altbootcmd`) depended on the previous slot
still being intact. The cost of this rule is a wasted boot into FACTORY on
the rare torn write, not a wrong app running.

A future hardening option, not adopted for v0.4 to keep the initial scope
tight: keep `slot.state` as two redundant, alternately-written copies with
sequence numbers, the same mitigation U-Boot uses for its own environment
(`CONFIG_SYS_REDUNDAND_ENVIRONMENT`, Appendix A) — worth revisiting if torn
`slot.state` writes turn out to be common in practice rather than a
theoretical edge case.

## 5. Supervisor probation and the three-rung ladder

Grounded in the actual code: `cmd/gosd-init/internal/boot/supervisor.go`'s
`Supervisor` today only restarts `/app` on exit with exponential backoff
(`backoff.go`) and resets that backoff once a run exceeds
`StableRunThreshold` (currently 30s) — it has no concept of a boot slot, and
`Run` in `sequence.go` resolves a single, fixed `AppPath` once at the start
of boot (today a hardcoded `/app` constant in `main.go`). This design
extends both, but doesn't replace either:

- **`AppPath` resolution becomes a function of `slot.state`,** resolved once
  at the same point `sequence.Run` already resolves `cfg`/`cmdline`
  (`sequence.go` lines ~93-116) — not a new architectural step, just what
  that existing single-resolution-pass now resolves. `Target=factory` →
  `/app` (unchanged); `Target=a`/`b` → `/boot/app/app.a`/`app.b`, after the
  `SHA256`/`Size` re-check from §3 (a mismatch is treated exactly like a
  probation failure, §below — immediate ladder escalation without even
  attempting to start it).
- **`Status=committed`** is today's existing behavior exactly as it already
  works: `Supervisor.Run` restarts forever with backoff, no rollback, no
  reboot — CLAUDE.md's existing model, untouched.
- **`Status=probation`** is new: a bounded stability window on top of the
  existing restart loop.
  - If `/app` completes one continuous run of at least a new
    `ProbationStableThreshold` (deliberately longer than the existing 30s
    `StableRunThreshold` — that constant answers "is this a crash loop
    right now," probation answers "do I trust this slot forever," which
    calls for materially more confidence on an unattended, remote device)
    — gosd-init writes `slot.state` (temp+fsync+rename, same protocol as
    §4) with `Status=committed`, and probation ends **permanently** for
    that slot. Every crash after this point is ordinary steady-state
    restart-forever, exactly as today, no matter how long after the update
    it happens. This is the direct answer to "how does probation interact
    with a legitimate crash long after an update": it can't roll back,
    because probation is one-shot and self-terminating — by the time a
    "long after" crash happens, there is no probation left to interact
    with.
  - If `/app` instead fails to reach that threshold within a bounded
    overall probation budget (bounded attempts or bounded wall-clock time,
    catching both a fast crash loop and a slow one that always dies just
    under the threshold) — gosd-init writes `slot.state` with
    `Target=Previous, Status=committed` (rung 2; skipping straight to
    `factory` if `Previous` is itself `factory` or empty, i.e. this was the
    first update ever pushed) and calls `deps.Rebooter.Reboot()` (already
    wired in `Deps`, currently used only by `sequence.go`'s `fatal()` path
    — this is a new caller, not new capability).
  - The reboot lands back at early boot, re-reads `slot.state`, and now
    resolves `AppPath` to the previous good slot with `Status=committed` —
    it does **not** get its own probation window, since it was already
    proven stable by an earlier update cycle.

**The three-rung ladder in full:** new slot (probation) → previous good
slot (already committed, no probation) → FACTORY (always committed, the
floor). Unlike the rejected design, none of this needs a firmware
mechanism, a kernel reboot triggered by hardware, or per-board code — it's
the same `Supervisor`/`sequence.Run` extension on every board.

**Grounded code changes this implies** (none touch `internal/image` or a
board profile):
- `cmd/gosd-init/internal/boot/supervisor.go`: a probation variant of the
  stability check in `runOnce()`.
- `cmd/gosd-init/internal/boot/sequence.go` / `main.go`: `AppPath` becomes
  resolved from `slot.state` instead of the current hardcoded `appPath`
  const; probation escalation calls the existing `deps.Rebooter`.
- `cmd/gosd-init/internal/boot/mounts.go`: `MountBootPartition` hardcodes
  `msRdOnly` today with no read-write path at all. The update endpoint
  needs `GOSD-BOOT` writable for the (short) duration of a push/activate
  call — proposed as a narrow remount-read-write-then-back-to-read-only
  around that window, preserving the exact discipline the rejected design
  already praised ("only the update flow itself, and only while actively
  committing, should ever open GOSD-BOOT for writing"), just scoped to
  `/boot/app/*` instead of a bootloader config file.
- A new package (proposed `cmd/gosd-init/internal/appslot`) owning
  `SlotState`'s (de)serialization and the fail-safe parse rule, analogous
  to how `internal/initcfg` owns `config.json`'s schema today, but for a
  file that's mutated at runtime rather than baked once at build time.

## 6. The update endpoint

Same overall shape as the rejected design's proposal — three endpoints,
still the second sanctioned network listener CLAUDE.md allows alongside
mDNS, still deliberately narrower than gokrazy's admin surface:

- `GET /update/info` — board ID, current `Target`/`Status`, free bytes on
  `GOSD-BOOT`, protocol version. No auth required (read-only, no secret
  material) — `gosd push` needs this before it's authenticated anything.
- `PUT /update` — streams a new app binary into the *inactive* slot
  (§4's push phase). Authenticated (§6.3). Returns the server-computed hash
  for the client's own sanity check.
- `POST /update/activate` — commits `slot.state` and reboots (§4's activate
  phase). Authenticated. Splitting this from `PUT /update` (rather than one
  combined endpoint) keeps "write" and "commit" as separately observable
  HTTP steps, matching the two-phase commit protocol itself, and gives a
  natural place to reject a stale activate (e.g. if `Generation` in the
  request doesn't match what's on disk, because a second push raced in).

### 6.1 Concurrent-push rejection

gosd-init is a single process; it keeps one in-memory flag ("a `PUT
/update` is currently streaming"). A second `PUT /update` while one is
in-flight is rejected with `409 Conflict` — this reuses gokrazy's own
`nonConcurrentUpdateHandler` naming/pattern, already identified as
transferring directly in the original research. A `PUT /update` that
arrives *after* a previous one completed but *before* `/activate` was
called is allowed (it simply overwrites the same inactive slot's `.tmp`/
real file again) — a useful, idempotent retry path if a client-side hash
check failed and the developer re-runs `gosd push`.

### 6.2 App-size limits

`GOSD-BOOT` is a fixed-size FAT32 partition shared with the kernel,
initramfs, and board boot config — this design does not hardcode a specific
byte budget (CLAUDE.md: don't bake stale figures into docs), but defines
the *mechanism*: `GET /update/info` reports current free bytes; `gosd push`
knows its own binary's size before it starts streaming and refuses to begin
unless free space is at least some safety multiple of that size (needs
headroom for the `.tmp` file to coexist with the old inactive-slot content
until rename — a push only ever touches the inactive slot, so the peak
extra usage is roughly one slot's worth of overlap, not the whole
partition). The server independently re-checks free space before accepting
the stream body and aborts with a clear 4xx rather than running out of
space mid-write, which on FAT risks compounding into exactly the kind of
partial-write corruption §4 is designed to contain.

### 6.3 Authn: per-image HMAC key baked at build time

Reused near-verbatim from the rejected design's §4.3 — the reasoning
doesn't depend on what's being authenticated, only that there's no TLS
(clocks start at 1970 until SNTP lands, `gosd-c8oj`) and no interactive
operator to type a password (`docs/runtime.md`'s "hand a friend an SD card"
positioning). What changes is *what* the HMAC covers:

- `gosd build` generates a random per-build key (32 bytes,
  `crypto/rand`), bakes it into `/etc/gosd/config.json` (extending
  `initcfg.Config` with an `UpdateKey` field, following the exact precedent
  of that file already being the build-time-baked config gosd-init reads),
  and writes the same key alongside the built image
  (`<appname>-<board>.img.update-key`) for `gosd push` to read.
- `PUT /update`'s `Authorization` header carries an HMAC-SHA256 computed
  **client-side, over the exact app binary bytes**, before streaming
  starts (both sides know the shared secret, so this doesn't require a
  round trip). The server verifies it as a **streaming** HMAC while the
  body arrives — never buffering the whole binary, consistent with both
  boards' RAM constraints — and only proceeds to `fsync`+rename (§4) on a
  match; a mismatch deletes the `.tmp` file and returns an error. This is
  the concrete answer to "signature/integrity check before activation":
  activation is strictly gated on a verified MAC over the precise bytes
  that get renamed into place, not merely on the TCP stream completing.
- `POST /update/activate`'s `Authorization` header carries an HMAC over its
  request body (the target slot + expected `Generation`), checked with
  `subtle.ConstantTimeCompare`, same constant-time precedent as the
  rejected design borrowed from gokrazy.
- **Same explicit residual risk as before, unchanged by this revision:**
  this authenticates "does the pusher hold the per-image key," not "is this
  connection encrypted" — a passive LAN observer can still see the payload
  in transit. Judged an acceptable v0.4 trade-off given mDNS-scoped LAN
  discovery is already the deployment assumption; written down as a
  trust-boundary caveat in shipped docs, not left implicit: **the update
  endpoint assumes a trusted LAN; it is not designed to be exposed to the
  open internet.**

## 7. `gosd push <host>` flow

```
gosd push <host> [--board <id>]
```

1. `GET /update/info` — board ID (sanity-checked against the binary about
   to be pushed), current `Target`, free space, protocol version.
2. Compute `HMAC-SHA256(key, appBinary)` using the key from
   `<appname>-<board>.img.update-key`. Note this is a **much cheaper**
   artifact than the rejected design's push flow, which had to rebuild an
   entire kernel+initramfs+boot-config bundle per push — an app-only update
   reuses none of `internal/pipeline.Assemble`'s image-writing machinery,
   only the existing cross-compiled app binary `gosd build` already
   produces.
3. Pre-flight size check against `/update/info`'s reported free space
   (§6.2); refuse locally rather than starting a doomed transfer.
4. `PUT /update`, streaming the binary with the HMAC header; abort (nothing
   committed) on a hash mismatch reported back.
5. `POST /update/activate`; the device commits `slot.state` and reboots.
6. Poll `GET /update/info` with a generous timeout, watching for `Target`
   to flip to the new slot and, subsequently, `Status` to flip from
   `probation` to `committed`. Report success only once probation clears;
   report "activated, probation pending" if the poll window closes before
   that; report a plain, non-alarmist timeout otherwise — as the rejected
   design already noted, a timeout here is inherently ambiguous (network
   partition vs. still-on-probation vs. already rolled back) and should be
   reported as such, not conflated with a confirmed failure.

## 8. Recommendation and v0.4 task breakdown

**Recommendation:** the app-slot scheme in §§2-7 — a single, board-agnostic
mechanism, scoped strictly to the app binary, layered entirely inside
gosd-init's existing `Supervisor`/boot sequence and the existing single
`GOSD-BOOT` partition. No per-board code, no `internal/image` layout
change, no board-profile changes at all.

Proposed v0.4 beans (titles + one-line scope; **not created — for JP to
review and create**):

1. **`GOSD-BOOT` app-slot store + commit protocol** — new package (proposed
   `cmd/gosd-init/internal/appslot`) owning `SlotState`'s format, the
   write-temp/fsync/rename helpers for both the app binary and
   `slot.state`, and the fail-safe-toward-factory parse rule (§§3-4).
2. **Supervisor probation + three-rung ladder** — extend
   `cmd/gosd-init/internal/boot/supervisor.go` and `sequence.go` with
   `slot.state`-driven `AppPath` resolution, the probation stability
   window, and reboot-mediated escalation via the existing `Rebooter`; add
   the narrow read-write remount to `mounts.go` (§5).
3. **Update HTTP endpoint + build-time HMAC key** — `GET /update/info`,
   `PUT /update`, `POST /update/activate` in gosd-init (the second
   sanctioned listener alongside mDNS, bean `gosd-r796`); extend
   `initcfg.Config` with `UpdateKey`, generated by `gosd build` (§6).
4. **`cmd/gosd`: `gosd push <host>` command** — the CLI-side flow in §7,
   reusing the existing cross-compiled app artifact rather than
   `pipeline.Assemble`.
5. **Docs: update contract** — extend `docs/runtime.md` with the
   operator-facing flow, the rollback/probation guarantees from §5, the
   size-budget mechanism, and the explicit LAN-trust-boundary caveat from
   §6.3.

Suggested sequencing: (1) has no dependencies and can start immediately;
(2) depends on (1) (needs a real slot store to read/write); (3) depends on
(1) and, for `/update/activate`'s reboot behavior, benefits from (2)
existing first; (4) depends on (3); (5) can start any time and should land
alongside (2)/(3).

## Appendix A: Considered and rejected: per-board boot-slot A/B

**Rejected by JP:** GoSD intends to support many boards over time, and
maintaining a distinct bootloader-level A/B mechanism per board (Pi
`tryboot`'s N-partition restructuring of `internal/image`, Radxa's U-Boot
env/bootcount configuration) does not scale as a per-board maintenance
burden. The app-slot scheme in the body of this document supersedes this
recommendation. The research below is preserved for its citations and
because parts of its reasoning (FAT's weak atomicity, gokrazy's two-phase
commit pattern, the "no rollback for a userspace crash loop that never
reboots the kernel" finding) still inform the accepted design above.

### A.1 What gokrazy does, and what transfers

Read directly from source (not blog summaries): `github.com/gokrazy/updater`
(client) and `github.com/gokrazy/gokrazy`'s `update.go`/`gokrazy.go`/
`authenticated.go` (server, i.e. what runs on the device).

**gokrazy's mechanism:**
- Two root partitions, used alternately. The *inactive* one is always the
  update target — the running system is never written to directly.
- HTTP push, PUT-based, one endpoint per destination: `/update/mbr`,
  `/update/root`, `/update/boot`, `/update/bootonly`, each backed by
  `nonConcurrentUpdateHandler` — streams the request body straight to a
  block device/file, hashing as it streams (SHA-256, or CRC32 if the client
  negotiates `X-Gokrazy-Update-Hash: crc32` via a `/update/features`
  capability-negotiation endpoint), and returns the hex digest for the
  client to verify (`updater.Target.StreamTo`).
- `/update/switch` (`nonConcurrentSwitchHandler`) rewrites `root=` in
  `cmdline.txt` (and a systemd-boot loader entry, if present) to point at
  the partition just updated — an immediate, no-confirmation switch.
- `/update/testboot` (`nonConcurrentTestbootHandler`) is the safer path: it
  does **not** touch `root=` at all. It rewrites `cmdline.txt` to add
  `gokrazy.try_boot_inactive=1`. On the **next** boot (still the *old*,
  known-good root, since `root=` wasn't touched), `gokrazy.go`'s
  `maybeSwitchToInactive()` sees that flag, rewrites `cmdline.txt` again —
  replacing the flag with `gokrazy.switch_on_boot=1` and *actually* pointing
  `root=` at the new partition — and only then does the real switch. This
  two-phase, boot-mediated commit (rather than an immediate rewrite-and-hope)
  is the closest thing gokrazy has to a "boot-success watchdog"; it buys one
  clean boot of the *old* environment to perform the switch safely, rather
  than doing it optimistically from the update client's HTTP request.
- Auth: `authenticated.go` — HTTP Basic auth, one password per gokrazy
  instance (`Update.HTTPPassword` in `config.json`), constant-time compared
  (`subtle.ConstantTimeCompare`). If no password is set, the update surface
  is reachable **only** via a Unix socket, verified by peer address — i.e.
  "no password" means "local only," never "open to the network."
- Update-payload integrity, not authenticity: the SHA-256/CRC32 hash
  protects against transport corruption; there's no signature over the
  payload, so authenticity rests entirely on the Basic-auth password.

**What transfers to GoSD directly:**
- Streaming PUT to the *inactive* slot with a returned/verified hash — this
  pattern requires no design change, just a home for it in gosd-init.
- The two-phase "mark, reboot, confirm-and-commit" pattern generalizes: GoSD
  should not flip "which slot boots" atomically from an HTTP handler either.
  Whether the safe commit point is boot-mediated (as gokrazy does) or purely
  firmware-mediated (as Pi `tryboot` does, see §A.2) depends on the board.
- The auth posture — "closed by default, single secret compared in constant
  time" — transfers as a floor, though the bean explicitly asks for a
  per-image baked key instead of an operator-typed password (see §A.4).

**What does not transfer:**
- There is no rootfs to swap. GoSD's A/B unit is the kernel+initramfs pair
  (and, on Radxa, U-Boot's chosen extlinux entry), not a partition mounted
  as `/`.
- gokrazy assumes one bootloader story is close enough across its supported
  boards that one cmdline-flag mechanism suffices. GoSD's two boards don't
  share a bootloader at all, so the "commit" step is necessarily
  board-specific (§A.2).
- gokrazy's model doesn't have to reconcile with a "no interactive surface,
  ever" constraint — its web UI is intentionally a small admin surface.
  GoSD's update endpoint has to be the *only* thing of its kind, deliberately
  narrower (§A.4).

Sources: [gokrazy/updater](https://github.com/gokrazy/updater) (`updater.go` —
`StreamTo`, `Switch`, `Testboot`), [gokrazy/gokrazy `update.go`](https://github.com/gokrazy/gokrazy/blob/main/update.go)
(`nonConcurrentUpdateHandler`, `nonConcurrentSwitchHandler`,
`nonConcurrentTestbootHandler`, `initUpdate`), [gokrazy/gokrazy `gokrazy.go`](https://github.com/gokrazy/gokrazy/blob/main/gokrazy.go)
(`maybeSwitchToInactive`, `switchRootPartition`, `enableTestboot`),
[gokrazy/gokrazy `authenticated.go`](https://github.com/gokrazy/gokrazy/blob/main/authenticated.go).

### A.2 Per-board mechanism

#### A.2.1 Raspberry Pi Zero 2 W: `tryboot`

Official docs, read directly (not paraphrased from a summary):
[`autoboot.txt` reference](https://github.com/raspberrypi/documentation/blob/master/documentation/asciidoc/computers/config_txt/autoboot.adoc)
and the [fail-safe OS updates (`tryboot`) section](https://github.com/raspberrypi/documentation/blob/master/documentation/asciidoc/computers/raspberry-pi/bootflow-eeprom.adoc).

Key facts, quoted:
- *"All Raspberry Pi models support `tryboot`, however, on Raspberry Pi 4
  Model B revision 1.0 and 1.1 the EEPROM must not be write protected."* —
  confirms Zero 2 W is covered; the EEPROM caveat is Pi-4-specific and
  irrelevant to our boards. `tryboot` is a firmware (`start.elf`/GPU-side)
  feature, not an EEPROM-bootloader-only feature — the Zero 2 W loads that
  same firmware from the FAT partition via `bootcode.bin`, it just does so
  without an EEPROM stage in front of it.
- `autoboot.txt` specifies `boot_partition` (which of up to 4 MBR partitions
  to boot from) and supports `[all]`/`[tryboot]` conditional sections.
- `tryboot_a_b=1` means the trial boot loads the **normal** `config.txt`
  from the trial partition rather than a separate `tryboot.txt`, so the
  A/B decision is made at the partition level, matching how we'd want to
  keep each slot self-contained.
- To trigger a trial boot: `sudo reboot "0 tryboot"` (or the numbered
  partition instead of `0` for "default"). The tryboot flag is **one-shot,
  firmware-held, and auto-clears on any reset that isn't that specific
  reboot call** — a raw power cycle or panic during the trial boot returns
  to `config.txt`/`[all]`, i.e. the previously-committed slot, with zero
  software involvement required. This is the single biggest advantage over
  a purely software rollback: it protects against failures *before*
  gosd-init ever runs (a corrupt initramfs, a kernel panic, a hang).
- Critically: *"Bootable partitions must be formatted as FAT12, FAT16 or
  FAT32 and contain a `start.elf` file... in order to be classed as
  bootable."* **Each slot must be its own bootable FAT partition** — Pi
  A/B is not "two files in one partition," it's two partitions, each
  carrying its own `bootcode.bin`/`start.elf`/`fixup.dat`, `config.txt`,
  `cmdline.txt`, kernel, and initramfs. That's a structural change to
  `internal/image`, which today writes exactly one FAT32 partition
  (`bootPartitionIndex = 1`, hardcoded in `image.go`).
- The official example update flow (quoted in full from the doc) confirms
  the commit step is a plain rewrite of `autoboot.txt`'s `[all]` section
  after the trial boot validates itself — no reboot is strictly required to
  commit, only to make the switch take effect.

Open question flagged, not resolved here: some Pi documentation states
`autoboot.txt` "must be on the first partition of the boot media." If that's
a hard requirement independent of `boot_partition`, partition 1 may need to
always host `autoboot.txt` even when partition 2 is the active/booting slot
— this needs verification against real hardware, and is moot now that this
mechanism is not being built.

**Design that was proposed:** two bootable FAT32 partitions, `GOSD-BOOT-A` /
`GOSD-BOOT-B`, each a complete, independent boot set. `autoboot.txt` on
partition 1 carries `tryboot_a_b=1` and the current/trial `boot_partition`.
An update writes a full new boot set into the *inactive* partition, then
triggers `reboot "0 tryboot"`. gosd-init in the trial boot decides whether
to commit (rewrite `autoboot.txt`, normal reboot).

#### A.2.2 Radxa Zero 3E: U-Boot distro-boot, `bootcount`/`altbootcmd`

U-Boot docs: [Boot Count Limit](https://docs.u-boot.org/en/latest/api/bootcount.html),
[PXE/extlinux fallback semantics](https://docs.u-boot.org/en/latest/usage/pxe.html),
and Bootlin's [ELCE 2022 "Implementing A/B System Updates with U-Boot"](https://bootlin.com/pub/conferences/2022/elce/opdenacker-implementing-A-B-system-updates-with-u-boot/src/opdenacker-implementing-A-B-system-updates-with-u-boot.tex).

Key facts:
- `bootcount`/`bootlimit`: if `upgrade_available` is set, U-Boot increments
  `bootcount` on each boot and saves it; once `bootcount > bootlimit`,
  U-Boot runs `altbootcmd` instead of `bootcmd`. Userspace (Linux, i.e.
  gosd-init) is responsible for resetting `bootcount`/`upgrade_available`
  to 0 once it considers the boot successful — U-Boot has no visibility
  into whether Linux/the app is actually healthy.
- `extlinux.conf` (the file GoSD already renders for Radxa per bean
  `gosd-gbsz`) natively supports a `default` label and a `fallback` label:
  *"the label named 'default' is... the first label 'pxe boot' attempts to
  boot, while the label named 'fallback' is treated as a fallback option
  that may be attempted should it be detected that booting of the default
  has failed to complete, for example via U-Boot's boot count limit
  functionality."* This means an A/B scheme on Radxa can live entirely
  **inside a single `extlinux.conf`, in a single boot partition** — no
  second partition needed, unlike the Pi. That was a real asymmetry between
  the two boards' proposed mechanisms.
- Bootlin's caveat, worth taking seriously for any failure model: *"saving
  the environment is far from being atomic. Multiple storage blocks need to
  be modified. Many things could go wrong."* Their mitigation is U-Boot's
  redundant-environment feature (two copies with sequence numbers, so a
  torn write leaves one valid copy) — `CONFIG_SYS_REDUNDAND_ENVIRONMENT`.
  (This is the precedent §4 of the accepted design cites for its own
  "redundant `slot.state` copies" future-hardening option.)

**What our v2026.04 build already has vs. needs:** checked directly against
the pinned `radxa-zero-3-rk3566_defconfig` (bean `gosd-d458`, already merged
into `build/boards/radxa-zero-3e/uboot/`): it sets no `CONFIG_ENV_IS_IN_*`,
no `CONFIG_BOOTCOUNT_LIMIT`, no `CONFIG_SYS_REDUNDAND_ENVIRONMENT`. Distro-
boot/bootstd itself is confirmed present (the bean's decision note: *"Do NOT
strip distro-boot/bootstd — we rely on it to find extlinux/extlinux.conf"*),
so `extlinux.conf` parsing with `default`/`fallback` labels should work
today, but **environment persistence and bootcount are not configured at
all** — right now there's nowhere durable for U-Boot to even remember
`bootcount` across a reboot.

**Design that was proposed:** one `GOSD-BOOT` partition, two labelled
entries in `extlinux.conf` (`default`/`fallback`, or two named slots
`gosd-a`/`gosd-b` with U-Boot env choosing which is `default`), each
pointing at its own `Image`/`.dtb`/`initramfs.cpio.zst` (e.g. under `/a/`
and `/b/` directories in the same partition). An update writes a new boot
set into the inactive slot's directory, updates `extlinux.conf` (or an env
var selecting the slot) plus `upgrade_available=1`/`bootcount=0`, and
reboots. If Linux never confirms, `bootlimit` is exceeded and U-Boot falls
back to `altbootcmd`/the fallback label automatically.

#### A.2.3 Asymmetry worth naming explicitly

Radxa's mechanism was materially cheaper to build: one partition, one file
format already generated, no `internal/image` layout change — just new
U-Boot config and two `extlinux.conf` entries. Pi's mechanism required
restructuring `internal/image` to write N independent bootable FAT
partitions and duplicating the entire boot file set per slot (kernel,
firmware, initramfs — likely 30-60MiB each way on the Zero 2W), a real
SD-card space and build-time cost on a board this small. In exchange, Pi's
`tryboot` gives a strictly stronger failure guarantee: rollback that works
even if the kernel itself fails to boot, with zero code required. This
per-board asymmetry — different partition layouts, different config
surfaces, different failure guarantees, on every board added from here — is
exactly the scaling cost that led to this whole approach being rejected in
favor of the app-slot scheme.

### A.3 Failure model (as it applied to boot-slot A/B specifically)

Power loss mid-transfer: both boards write only to the inactive slot, so an
interrupted transfer leaves the active slot untouched and nothing commits
until the transfer is fully verified — the easy case, no new mechanism
needed. Power loss during commit: Pi's `autoboot.txt` rewrite and Radxa's
U-Boot env save are each, in their own way, a small filesystem/env write
that isn't provably atomic (§A.2's Bootlin citation); the mitigations were
the same shape as the accepted design's (keep the commit payload small,
prefer built-in redundancy where available, fail toward the last-known-good
choice on a garbled read) — this is the direct ancestor of §4's fail-safe
parsing rule.

**The one failure mode the app-slot scheme cannot cover, conceded plainly:**
a userspace crash loop was already found, in this research, to be invisible
to both `tryboot` (one-shot, consumed by the boot that's already running)
and `bootcount` (only increments across real kernel reboots) —
`cmd/gosd-init/internal/boot/supervisor.go`'s `Supervisor` never reboots the
kernel, only restarts the process. That finding is exactly what motivated
the "update probation" concept in the first place; it now lives in the
accepted design's §5, unchanged in spirit, just triggered by `slot.state`
instead of a board-specific firmware flag.

### A.4 What the rejected design's authn section already got right

Its reasoning (no TLS due to the epoch-clock problem, no operator password
due to no interactive setup, per-image HMAC key baked at build time,
constant-time comparison) did not depend on *what* was being pushed — it
carries over to the accepted design's §6.3 essentially unchanged.

## Appendix B: Escape hatch — kexec-chooser for kernel-level OTA

Not adopted. Documented here in case kernel/initramfs OTA ever becomes a
hard requirement (e.g. a security patch that can't wait for a physical
reflash), as a board-agnostic alternative to reopening Appendix A's
per-board approach.

**The idea:** `kexec_file_load(2)` lets an already-running Linux kernel
load and jump directly into a *new* kernel+initramfs pair without going
through firmware/bootloader at all — it's a Linux syscall, not a per-board
mechanism, so it sidesteps the entire `tryboot`-vs-`bootcount` asymmetry
that made per-board A/B expensive. A small "chooser" stage (either the start
of `gosd-init` itself, or a distinct first-stage init ahead of it) would
read a generalized version of `slot.state` naming a kernel/initramfs pair,
and if it names something other than what's currently running, `kexec` into
it — with its own probation/ladder logic mirroring §5, one layer lower
(kernel-slot state instead of app-slot state).

**Costs, why this isn't adopted now:**
- **~1-2 seconds of added boot time**, every boot: `kexec` reloads and
  jumps to a second kernel image after firmware has already loaded and
  booted the first one — strictly additive over a firmware that loads the
  right kernel directly, once, which is what happens today.
- **Real added complexity:** `CONFIG_KEXEC_FILE` in the kernel build,
  a chooser stage that has to be extremely reliable in its own right (a bug
  in the chooser can brick a device as badly as a bug in a shared
  bootloader would — there's no firmware fallback underneath it), and its
  own probation/rollback bookkeeping distinct from the app-level one in §5.
- **It gives up `tryboot`/`bootcount`'s best property.** `kexec` only runs
  *after* a kernel has already booted successfully once — it has no answer
  for "the new kernel panics before it can run anything," which is
  arguably the single most valuable guarantee `tryboot`/`bootcount` provide
  and this design would be reaching for kexec specifically to replace them.

Per §0, the kernel/initramfs/bootloader remain reflash-only for now. This
appendix exists so that if that ever needs to change, the option space
(reopen per-board A/B vs. build this chooser) is already written down
rather than re-researched from scratch.

## Open questions for JP / follow-up beans

- Exact value for `ProbationStableThreshold` (§5) — deliberately left as an
  implementation decision for whoever picks up task 2, not a research
  question; it should be materially longer than the existing 30s
  `StableRunThreshold` but doesn't need to be resolved in this document.
- Exact free-space safety multiple for the size-budget check (§6.2) —
  same status, an implementation decision for task 1/3, not blocking this
  doc.
- Whether `slot.state` should adopt the redundant-copy hardening noted at
  the end of §4 now or only if torn writes prove to be a real-world problem
  — left open, leaning toward "not yet" to keep v0.4's scope tight.
- Whether `gosd push`'s per-image key should be regenerable/rotatable
  post-flash (e.g. via `gosd.toml`, bean `gosd-tds2`, once that lands) or is
  genuinely fixed for the image's lifetime — carried over unresolved from
  the rejected design, unaffected by this revision.

## Acceptance

- [ ] Doc reviewed (JP)
- [ ] Follow-up beans created for the chosen design (JP)

## Summary of Changes

Rewrote `docs/design/ab-updates.md` after JP rejected the per-board
boot-slot recommendation (Pi `tryboot`, Radxa U-Boot `bootcount`) on
scaling grounds: GoSD intends to support many boards, and a distinct
bootloader-level A/B mechanism per board is a maintenance cost that grows
with every board added. The new recommendation is a single, board-agnostic
**app-slot** scheme:

- Explicit scope decision (§0): OTA updates replace only the app; the
  kernel/initramfs/bootloader are reflash-only.
- The initramfs-baked `/app` becomes the immutable FACTORY image; two new
  slot files (`app.a`/`app.b`) live inside the existing single `GOSD-BOOT`
  partition — no partition-table change, unlike the rejected design.
- A two-file (app binary, then `slot.state`) write-temp/fsync/rename commit
  protocol, with a precise table of what survives power loss at each step
  and a fail-safe-toward-FACTORY parsing rule for a garbled `slot.state`.
- A new Supervisor probation mode and three-rung fallback ladder (new slot
  → previous good slot → FACTORY), grounded in the actual
  `cmd/gosd-init/internal/boot` code: `Supervisor`/`backoff.go`'s existing
  30s `StableRunThreshold` is distinct from the new, longer
  `ProbationStableThreshold`; escalation reuses the already-wired
  `deps.Rebooter` instead of adding new capability; `mounts.go`'s
  hardcoded read-only mount needs a narrow read-write window during
  updates.
- The update endpoint (info/update/activate) and `gosd push` flow, reusing
  and adapting the rejected design's HMAC-at-build-time authn reasoning
  (still no TLS, still no operator password, same LAN-trust-boundary
  caveat), now gating activation on a streaming HMAC verified before any
  rename — plus concurrent-push rejection and a free-space preflight
  mechanism for the FAT size budget.
- The rejected per-board research is preserved as Appendix A (with its
  citations intact) and the kexec-chooser idea is introduced fresh as
  Appendix B, a documented but unadopted escape hatch if kernel-level OTA
  ever becomes a hard requirement.
- Replaced the previous nine-bean v0.4 breakdown with five beans scoped to
  the app-slot scheme (proposed only, not created, per the bean's
  acceptance requiring JP's review first).

Bean `gosd-v2w1` stays in-progress: the acceptance checklist (doc reviewed;
follow-up beans created) still requires JP and remains unchecked.
