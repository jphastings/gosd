# Design spike: over-the-network app updates (A/B scheme)

Status: design only, nothing in this document is built. Tracked by bean
`gosd-v2w1`. Target: informing a v0.4 task breakdown.

## 0. Why this is harder for GoSD than for gokrazy

gokrazy's A/B update model swaps a **root filesystem** partition underneath a
kernel and boot partition that don't change. GoSD has no root filesystem at
all: per `docs/runtime.md` and `internal/image/image.go`, the entire OS is a
kernel + initramfs pair, loaded straight off the FAT32 `GOSD-BOOT` partition,
with the app's own binary baked into that initramfs
(`internal/pipeline.Assemble`, `internal/initramfs`). There is no
`pivot_root`/`switch_root` step and, today, no second partition at all — the
locked layout in `internal/image/image.go` is MBR + one 256MiB FAT32
partition (`GOSD-BOOT`) starting at 16MiB.

That means "A/B" for GoSD can't be "swap the rootfs, keep the kernel." The
**unit of update has to be the kernel+initramfs pair itself** (the bean's own
framing is right: "two kernel+initramfs pairs on GOSD-BOOT"). And because
Pi Zero 2 W and Radxa Zero 3E boot in fundamentally different ways — the Pi
has no bootloader at all (the GPU ROM loads `kernel8.img` directly per
`internal/boards/pizero2w/board.go`), the Radxa boots through mainline
U-Boot (bean `gosd-d458`) — the two boards need genuinely different A/B
mechanisms. There is no single scheme that covers both; gokrazy gets away
with one scheme partly because it targets a wider, more homogeneous set of
boards and always keeps a persistent root filesystem as the swap unit.

## 1. What gokrazy does, and what transfers

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
  firmware-mediated (as Pi `tryboot` does, see §2) depends on the board.
- The auth posture — "closed by default, single secret compared in constant
  time" — transfers as a floor, though the bean explicitly asks for a
  per-image baked key instead of an operator-typed password (see §4).

**What does not transfer:**
- There is no rootfs to swap. GoSD's A/B unit is the kernel+initramfs pair
  (and, on Radxa, U-Boot's chosen extlinux entry), not a partition mounted
  as `/`.
- gokrazy assumes one bootloader story is close enough across its supported
  boards that one cmdline-flag mechanism suffices. GoSD's two boards don't
  share a bootloader at all, so the "commit" step is necessarily
  board-specific (§2).
- gokrazy's model doesn't have to reconcile with a "no interactive surface,
  ever" constraint — its web UI is intentionally a small admin surface.
  GoSD's update endpoint has to be the *only* thing of its kind, deliberately
  narrower (§4).

Sources: [gokrazy/updater](https://github.com/gokrazy/updater) (`updater.go` —
`StreamTo`, `Switch`, `Testboot`), [gokrazy/gokrazy `update.go`](https://github.com/gokrazy/gokrazy/blob/main/update.go)
(`nonConcurrentUpdateHandler`, `nonConcurrentSwitchHandler`,
`nonConcurrentTestbootHandler`, `initUpdate`), [gokrazy/gokrazy `gokrazy.go`](https://github.com/gokrazy/gokrazy/blob/main/gokrazy.go)
(`maybeSwitchToInactive`, `switchRootPartition`, `enableTestboot`),
[gokrazy/gokrazy `authenticated.go`](https://github.com/gokrazy/gokrazy/blob/main/authenticated.go).

## 2. Per-board mechanism

### 2.1 Raspberry Pi Zero 2 W: `tryboot`

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
— this needs verification against real hardware before implementation
(candidate v0.4 task, see §5).

**Design for GoSD:** two bootable FAT32 partitions, `GOSD-BOOT-A` /
`GOSD-BOOT-B` (naming per CLAUDE.md's `GOSD-*` FAT label convention), each a
complete, independent boot set. `autoboot.txt` on partition 1 carries
`tryboot_a_b=1` and the current/trial `boot_partition`. An update writes a
full new boot set into the *inactive* partition, then triggers
`reboot "0 tryboot"`. gosd-init in the trial boot decides whether to commit
(rewrite `autoboot.txt`, normal reboot) — see §3 for exactly when.

### 2.2 Radxa Zero 3E: U-Boot distro-boot, `bootcount`/`altbootcmd`

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
  second partition needed, unlike the Pi. That's a real asymmetry worth
  weighing in the recommendation (§4).
- Bootlin's caveat, worth taking seriously for our failure model (§3):
  *"saving the environment is far from being atomic. Multiple storage
  blocks need to be modified. Many things could go wrong."* Their
  mitigation is U-Boot's redundant-environment feature (two copies with
  sequence numbers, so a torn write leaves one valid copy) —
  `CONFIG_SYS_REDUNDAND_ENVIRONMENT`.

**What our v2026.04 build already has vs. needs:** checked directly against
the pinned `radxa-zero-3-rk3566_defconfig` (bean `gosd-d458`, already merged
into `build/boards/radxa-zero-3e/uboot/`): it sets no `CONFIG_ENV_IS_IN_*`,
no `CONFIG_BOOTCOUNT_LIMIT`, no `CONFIG_SYS_REDUNDAND_ENVIRONMENT`. Distro-
boot/bootstd itself is confirmed present (the bean's decision note: *"Do NOT
strip distro-boot/bootstd — we rely on it to find extlinux/extlinux.conf"*),
so `extlinux.conf` parsing with `default`/`fallback` labels should work
today, but **environment persistence and bootcount are not configured at
all** — right now there's nowhere durable for U-Boot to even remember
`bootcount` across a reboot. This is a concrete, scoped v0.4 config change
(§5), not a research question: enable environment storage (most likely
`CONFIG_ENV_IS_IN_MMC` at a fixed offset in the same unpartitioned gap that
already carries `idbloader.img`/`u-boot.itb`, or `CONFIG_ENV_IS_IN_FAT` on
`GOSD-BOOT` — needs a size/offset decision, not a re-architecture),
`CONFIG_SYS_REDUNDAND_ENVIRONMENT`, and `CONFIG_BOOTCOUNT_LIMIT`.

**Design for GoSD:** one `GOSD-BOOT` partition, two labelled entries in
`extlinux.conf` (`default`/`fallback`, or two named slots `gosd-a`/`gosd-b`
with U-Boot env choosing which is `default`), each pointing at its own
`Image`/`.dtb`/`initramfs.cpio.zst` (e.g. under `/a/` and `/b/` directories
in the same partition — cheap, since it's just more files in the FAT
partition GoSD already writes, not a new partition). An update writes a new
boot set into the inactive slot's directory, updates `extlinux.conf` (or an
env var selecting the slot) plus `upgrade_available=1`/`bootcount=0`, and
reboots. If Linux never confirms, `bootlimit` is exceeded and U-Boot falls
back to `altbootcmd`/the fallback label automatically.

### 2.3 Asymmetry worth naming explicitly

Radxa's mechanism is materially cheaper to build: one partition, one file
format we already generate, no `internal/image` layout change — just new
U-Boot config and two `extlinux.conf` entries. Pi's mechanism requires
restructuring `internal/image` to write N independent bootable FAT
partitions and duplicating the entire boot file set per slot (kernel,
firmware, initramfs — likely 30-60MiB each way on the Zero 2W), which is a
real SD-card space and build-time cost on a board this small. In exchange,
Pi's `tryboot` gives a strictly stronger failure guarantee: rollback that
works even if the kernel itself fails to boot, with zero code required.
Radxa's `bootcount` gives the same "zero code required" property but only
because we can enable it in firmware config, whereas a Linux-userspace-only
scheme (rewriting a plain file with no firmware cooperation at all) would
have no answer for "the new kernel/initramfs panics before gosd-init runs."
This is why §4 recommends leaning on each board's purpose-built mechanism
rather than inventing one uniform scheme that ignores both.

## 3. Failure model

### 3.1 Power loss mid-transfer (writing the new boot set)

Both boards: the update payload is written **only** into the currently
*inactive* slot/directory — the active, currently-booting slot's files are
never opened for writing during a transfer. A power loss here leaves the
inactive slot in an unknown/partial state, but the machine reboots straight
back into the untouched active slot, because nothing has been committed
yet (no `autoboot.txt`/`extlinux.conf`/env change happens until the
transfer is hashed and verified in full — mirroring gokrazy's
verify-before-switch ordering). This case is the easy one and requires no
new mechanism beyond "commit is a separate, later step from transfer."

### 3.2 Power loss during the commit step itself

This is the step gokrazy's two-phase testboot dance, Pi's `tryboot` flag,
and U-Boot's `bootcount` are all, in their own way, designed to survive —
but "designed to survive" needs an honest caveat about the medium it's
built on.

- **Pi**: the commit is a rewrite of `autoboot.txt` (≤512 bytes). A power
  loss mid-write of that file is a FAT write, not a firmware-flag write, so
  it inherits FAT's atomicity limits (§3.3) — small mitigating factor: at
  ≤512 bytes it's likely to occupy a single allocation unit, making a torn
  write less likely in practice, but "less likely" is not "impossible," and
  nothing in the FAT32 spec promises single-sector write atomicity across a
  power cut. A worst case here (a garbled `autoboot.txt`) is not
  necessarily fail-safe — the firmware's parser behavior on a corrupt file
  needs to be checked before shipping this (v0.4 task).
- **Radxa**: the commit is a U-Boot environment save. Bootlin's own
  documented caveat applies directly: env saves are multi-block and *"far
  from atomic."* `CONFIG_SYS_REDUNDAND_ENVIRONMENT` (two copies, sequence
  numbers, older copy kept on a failed write) is the standard mitigation
  and should be treated as required, not optional, for this design — it's
  the one piece of U-Boot's own reliability story built exactly for this
  failure.

**Honest summary: FAT/plain-file commits are best-effort, not provably
atomic.** Neither board gives us a transactional filesystem. The
mitigations available are: (a) keep the commit payload as small as
possible (a boolean-ish flag, not a large config file), (b) prefer a
mechanism with its own redundancy built in (U-Boot's redundant env) over a
bare file rewrite where one exists, and (c) always be able to detect "the
commit file looks garbled" at the next boot and fail toward the
last-known-good slot rather than an undefined one. None of this is a
guarantee — it should be documented as a known residual risk, not sold as
solved.

### 3.3 FAT corruption in general

Cited above (§ research): FAT32 has no journal, keeps two copies of the FAT
table itself, and a power cut between updating one FAT copy and the other
(or between writing file data and the directory entry that references it)
can leave the two FAT copies inconsistent, or a directory entry pointing at
clusters that were never written. Write-then-rename is the standard
mitigation for "don't corrupt the file I'm replacing" (never truncate/edit
the live file in place; write to a new name, then rename over the old one)
— but **rename itself is a metadata operation on the same non-journaled
filesystem, so it is not power-loss-atomic either**; it reduces the window
and the blast radius (the old file is still intact if the crash happens
before the rename lands) but does not eliminate the risk the same way a
copy-on-write or journaled filesystem would. GOSD-BOOT is mounted read-only
by gosd-init in normal operation (`docs/runtime.md`) precisely because nothing
should be writing to it outside a deliberate, narrow update window — that
existing discipline is worth preserving: only the update flow itself (and
only while actively committing) should ever open GOSD-BOOT for writing.

### 3.4 App that boots but crashes — watchdog + rollback

This is the case with no existing analog in gosd-init today (confirmed:
`cmd/gosd-init/internal/boot/supervisor.go`'s `Supervisor` is a
crash-**restart** loop with exponential backoff and a `StableRunThreshold`
of 30s — it never reboots the kernel, and it has no concept of a boot
slot to confirm or roll back). That matters because both `tryboot` and
`bootcount` are triggered by **kernel reboots**, not by a userspace process
restarting — a crash-looping `/app` that the supervisor keeps quietly
restarting will never increment `bootcount` or fail a `tryboot` trial,
because from the firmware's point of view the machine only booted once and
is still running.

**Design implication:** gosd-init needs a new, narrow "update probation"
mode, active only for the boot(s) immediately following an update:
- If `/app` reaches `StableRunThreshold` (or a similar, possibly longer,
  post-update-specific threshold) without crashing, gosd-init performs the
  board's commit step (Pi: rewrite `autoboot.txt`'s `[all]` section;
  Radxa: `bootcount=0`, `upgrade_available=0`, promote the slot to
  `default`) and probation ends — subsequent crashes behave exactly as
  they do today (restart forever, per CLAUDE.md's existing model, no
  further rollback risk since we're back to "steady state").
- If `/app` instead crash-loops (fails to reach the stability threshold
  within a bounded number of attempts or a bounded wall-clock budget)
  **during probation**, gosd-init must escalate to an actual kernel reboot
  rather than continuing to restart the process in place — otherwise
  neither `tryboot` (one-shot, already consumed by this boot) nor
  `bootcount` (only increments across real reboots) ever gets the signal
  that this update is bad. On Pi, that reboot without a prior commit lands
  back on `config.txt`/`[all]`, i.e. the previous slot, automatically. On
  Radxa, it's one more increment toward `bootlimit`, then `altbootcmd`
  takes over automatically.
- A kernel panic, hang, or corrupt initramfs — the case *before* gosd-init
  can run at all — needs no new gosd-init logic: this is exactly what
  `tryboot`'s one-shot firmware flag and `bootcount`'s firmware-level
  counting exist to catch without any cooperating software.

This means the "watchdog" GoSD needs is not a hardware watchdog device
(none is wired up anywhere in the repo today — confirmed by grep) but a
software one living inside the existing `Supervisor`, whose only new
behavior is "escalate to reboot, don't just restart the process" while on
probation, plus a small persisted "which slot, and are we on probation"
marker it reads at early boot (a natural extension of the `gosd.board=`
cmdline-param pattern already read in `initcfg.ParseCmdline` — e.g.
`gosd.slot=a|b`).

## 4. Developer push flow and authentication

### 4.1 `gosd push <host>`

Shape (not implemented, this is the proposed CLI contract):

```
gosd push <host> [--board <id>]
```

1. CLI connects to `http://<host>.local/update/info` (or a plain IP/hostname
   — mDNS resolution is bean `gosd-r796`, not yet built, so `<host>` should
   accept a plain address too) and reads back the target's board ID and
   currently-active slot, mirroring gokrazy's `/update/features`
   negotiation. This lets `gosd push` build the *right* artifact (board is
   already known at `gosd build` time; the *slot* to target is not, and
   must come from the device).
2. CLI builds the same kernel+initramfs+board-boot-config bundle
   `gosd build` produces (reusing `internal/pipeline.Assemble` machinery)
   for the *inactive* slot, computing a SHA-256 as it streams — matching
   gokrazy's `StreamTo` pattern.
3. `PUT /update` streams the bundle to the device, which writes it into the
   inactive slot's files and returns the hex digest it computed
   server-side; the CLI aborts (not committing anything) on a mismatch.
4. `POST /update/testboot` tells the device to arm the trial boot
   (Pi: `reboot "0 tryboot"`; Radxa: set `upgrade_available=1`,
   `bootcount=0`, point `default`/env at the new slot) — the device reboots
   itself; the CLI does not need a separate `/reboot` call for the normal
   path (unlike gokrazy, which separates `/update/switch`+`/reboot`; GoSD's
   commit is inherently boot-mediated on both boards, so "arm and reboot"
   can be one step).
5. CLI polls `/update/info` (with a generous timeout) to observe the device
   come back with the new slot marked committed, and reports success/
   failure/timeout plainly to the developer. A timeout here is
   **ambiguous** by nature (network partition vs. rollback vs. still
   booting) and should be reported as such, not conflated with a confirmed
   failure.

### 4.2 The update endpoint's surface — minimal, by the bean's own constraint

CLAUDE.md is explicit: gosd-init has no interactive surface, ever, and the
only network listeners are mDNS and "later, the explicitly-designed update
endpoint." That endpoint is therefore the one sanctioned exception and
should be scoped as narrowly as gokrazy's is broad. Proposed surface —
deliberately smaller than gokrazy's (no `/divert`, no arbitrary
`/uploadtemp`, no `/poweroff`, no service-list/service-inspection UI, no
raw MBR write):

- `GET /update/info` — board ID, active slot, protocol/version, no auth
  required (read-only, no secret material in the response) — needed for
  `gosd push` to target the right slot before it has authenticated anything.
- `PUT /update` — streams the bundle to the inactive slot; authenticated
  (§4.3); returns the server-computed hash.
- `POST /update/testboot` — arms the trial boot and reboots; authenticated.

Three endpoints, no more. Nothing about reading logs, restarting the app on
demand, or inspecting device state beyond the one `/update/info` GET that
the push flow itself needs — anything else is explicitly out of scope so
this doesn't quietly grow into the "no interactive surface" carve-out
CLAUDE.md rules out.

### 4.3 Authn: per-image key baked at build time

The bean poses this as an open question ("per-image key baked at build?")
rather than a locked decision — proposed answer, reasoning included:

- **Why not gokrazy's operator-typed password:** that requires a human to
  choose and remember a secret per device, which cuts against GoSD's
  "hand a friend an SD card" positioning (`docs/runtime.md`) — there's
  often no interactive setup step at all.
- **Why not TLS:** `docs/runtime.md` is explicit that neither board has a
  battery-backed clock and SNTP isn't built yet (bean `gosd-c8oj`) — a
  freshly-booted device's clock reads 1970, so any certificate-validity
  check would fail exactly when a first update might plausibly happen
  (right after initial flash). TLS could layer on *later* once SNTP lands,
  but shouldn't be the only authn story now.
- **Proposed scheme:** `gosd build` generates a random per-build symmetric
  key (e.g. 32 bytes from `crypto/rand`), bakes it into the image's
  `/etc/gosd/config.json` (extending `initcfg.Config`, following the exact
  precedent of that file already being the build-time-baked config
  gosd-init reads) under something like `Config.UpdateKey`, and writes the
  same key to a local file next to the built image (e.g.
  `<appname>-<board>.img.update-key`) for the developer's `gosd push` to
  read. `PUT /update` and `POST /update/testboot` require an
  `Authorization` header carrying an HMAC-SHA256 over the request (body
  hash + a nonce/timestamp to prevent replay) keyed by that secret, checked
  with `subtle.ConstantTimeCompare` — directly reusing gokrazy's
  constant-time comparison precedent, upgraded from "compare a password" to
  "verify a MAC" since there's no TLS channel to lean on and no
  interactive operator to type anything.
- **Explicit residual risk, stated plainly rather than glossed over:**
  this authenticates "does the pusher know the per-image key," not "is
  this connection encrypted" — a passive network observer on the same LAN
  segment can see the update payload in transit even if they can't forge a
  valid push. Given gosd-init has no other listener besides mDNS and this
  endpoint, and the realistic deployment is a LAN/local network (mDNS
  discovery implies that scope already), this is judged an acceptable v0.4
  trade-off, but it should be written down as a trust-boundary assumption
  in the shipped docs, not left implicit: **the update endpoint assumes a
  trusted LAN; it is not designed to be exposed to the open internet.**

## 5. Recommendation and v0.4 task breakdown

**Recommendation:** build board-native A/B — Pi `tryboot` across two full
`GOSD-BOOT-A`/`GOSD-BOOT-B` partitions, Radxa `bootcount`+dual-`extlinux.conf`
-entry inside the single existing `GOSD-BOOT` partition — rather than
inventing one shared software-only scheme, because each board's
purpose-built mechanism gives rollback protection a software-only scheme
can't (surviving a kernel that never even reaches gosd-init), at the cost
of a real but bounded restructuring of `internal/image` and the U-Boot
build config. Pair both with a new "update probation" mode in gosd-init's
existing `Supervisor` (escalate to a real reboot, not just a process
restart, when an updated app fails to stabilize) since neither firmware
mechanism can see a userspace crash loop that never reboots the kernel. A
useful side effect: because `GOSD-DATA` (bean `gosd-xelb`, not yet built)
is planned as a separate partition untouched by a kernel+initramfs slot
swap, this design answers `gosd-xelb`'s deferred "does data survive an
update" question for free — an A/B *update* (as opposed to a full
`gosd build` reflash) never touches `GOSD-DATA` at all, so persisted data
naturally survives it.

Proposed v0.4 beans (titles + one-line scope; **not created — for JP to
review and create**):

1. **`internal/image`: N-partition layout support** — generalize
   `image.Spec`/`Write` beyond the current hardcoded single boot partition
   so a board profile can describe multiple named, independently-bootable
   FAT32 partitions (needed for Pi A/B; Radxa doesn't need this one).
2. **Pi Zero 2W: `tryboot` A/B board profile** — `GOSD-BOOT-A`/
   `GOSD-BOOT-B`, `autoboot.txt` template (`tryboot_a_b=1`,
   `boot_partition`), duplicate per-slot boot file sets; verify on real
   hardware whether `autoboot.txt` must live on partition 1 specifically
   and how the firmware behaves on a corrupt `autoboot.txt`.
3. **Radxa Zero 3E: U-Boot env + bootcount config** — add
   `CONFIG_ENV_IS_IN_{MMC,FAT}` (pick a location — likely the existing
   unpartitioned gap, alongside `idbloader.img`/`u-boot.itb`),
   `CONFIG_SYS_REDUNDAND_ENVIRONMENT`, `CONFIG_BOOTCOUNT_LIMIT` to the
   pinned U-Boot v2026.04 build; verify `bootcount`/`altbootcmd` actually
   fire on hardware.
4. **Radxa Zero 3E: dual-slot `extlinux.conf`** — extend the bean-`gosd-gbsz`
   template to two labelled entries (`default`/`fallback` or two named
   slots) each pointing at per-slot `Image`/`.dtb`/`initramfs.cpio.zst`
   paths within the single `GOSD-BOOT` partition.
5. **gosd-init: update-probation supervisor mode** — extend
   `cmd/gosd-init/internal/boot/supervisor.go` with a bounded post-update
   probation window that escalates to a real kernel reboot (not just a
   process restart) on repeated app failure, and performs the board-specific
   commit step (`autoboot.txt` rewrite / `bootcount`+env reset) once the app
   is stable; add `gosd.slot=`/persisted-marker plumbing alongside the
   existing `gosd.board=`/`gosd.debug` cmdline parsing in `initcfg`.
6. **gosd-init: update HTTP endpoint** — `GET /update/info`,
   `PUT /update`, `POST /update/testboot`, HMAC-authenticated per §4.3; the
   second sanctioned network listener alongside mDNS (bean `gosd-r796`).
7. **`gosd build`: per-image update key generation** — random per-build
   HMAC key baked into `/etc/gosd/config.json` (extending `initcfg.Config`)
   and written alongside the output image for the developer to keep.
8. **`cmd/gosd`: `gosd push <host>` command** — the CLI-side flow in §4.1,
   reusing `internal/pipeline.Assemble` to build the inactive-slot bundle.
9. **Docs: A/B update contract** — extend `docs/runtime.md` (or a new page)
   with the operator-facing update flow, the rollback/failure guarantees
   from §3, and the explicit LAN-trust-boundary caveat from §4.3.

Suggested sequencing: (1)-(4) are independent per-board plumbing and can
proceed in parallel once someone reviews this doc; (5) depends on (2)/(4)
existing (it needs a real slot/commit mechanism to call into); (6)-(8) are
the push/pull path and depend on (5); (9) can start any time and should
land alongside whichever of (2)/(4) ships first.

## Open questions for JP / follow-up beans

- Does `autoboot.txt` have to live on MBR partition 1 specifically,
  independent of which partition is currently active? (Flagged in §2.1,
  needs hardware verification — folded into task 2 above rather than
  blocking this doc.)
- Exact size/offset for U-Boot's environment on Radxa — the unpartitioned
  gap already holds `idbloader.img` (LBA 64) and `u-boot.itb` (LBA 16384);
  needs a size audit to pick a non-overlapping env offset, or a decision to
  use `CONFIG_ENV_IS_IN_FAT` instead and skip the gap entirely.
- Whether `gosd push`'s per-image key should be regenerable/rotatable
  post-flash (e.g. via `gosd.toml`, bean `gosd-tds2`, once that lands) or
  is genuinely fixed for the image's lifetime — left for whoever picks up
  task 7.

## Acceptance

- [ ] Doc reviewed (JP)
- [ ] Follow-up beans created for the chosen design (JP)

## Summary of Changes

Added `docs/design/ab-updates.md`: a design-only spike answering the four
questions in bean `gosd-v2w1` — what transfers from gokrazy's A/B/HTTP-push/
testboot model, a board-specific mechanism for Pi (`tryboot`, two bootable
FAT partitions) and Radxa (U-Boot `bootcount` + dual `extlinux.conf`
entries in the existing single partition), an honest failure model covering
power loss mid-transfer, power loss mid-commit, FAT's non-atomicity, and a
crash-looping app (which needs a new gosd-init "probation" mode since
neither firmware mechanism sees a userspace restart loop that never reboots
the kernel), a `gosd push`/update-endpoint design with a per-image
HMAC-key authn story, and a proposed (not created) v0.4 task breakdown of
nine beans. No code changed; `internal/image`, the Pi/Radxa board profiles,
gosd-init's supervisor, and `internal/initcfg` are all identified as needing
changes but none were touched.
