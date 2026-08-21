# Runtime contract for GoSD apps

This page is the mental model a Go developer needs to write an app that
behaves well on a GoSD image. It describes what actually exists on `main`
today; where something is planned but not yet built, that's called out
explicitly rather than described as if it worked.

Your app is cross-compiled with `CGO_ENABLED=0` for `GOOS=linux` —
`GOARCH` (and `GOARM`, where it applies) is chosen per board, see
`internal/build` and `internal/boards.Arch` — and copied into the image
as `/app`. There is no other userspace: no shell, no init system beyond
`gosd-init` itself, no package manager, no SSH. Whatever your Go binary
does is the whole system.

## At a glance

- **One process.** `gosd-init` is PID 1; it starts and supervises `/app`
  forever. Nothing else on the image is interactive: no shell, SSH, or
  remote debug access exists anywhere.
- **Networking, WiFi, DNS, and the clock all come up *after* `/app`
  starts**, asynchronously. Never assume connectivity or a correct clock
  at process start — retry instead of treating an early failure as fatal.
- **The root filesystem is RAM-backed and gone on reboot.** Of the image
  itself, only `/data` (opt-in, FAT32 by default or ext4, a fixed path)
  survives power loss — plus whatever you store via the `emmc`/`disk`
  packages. Built with
  `--data-size=expand`, `/data` — plus a hand-edited hostname, WiFi, or
  `[env]` value — also survives a *reflash* to a newer version; a
  fixed-size `--data-size` does not (see "Persistent storage: `/data`").
- **Logging is stdout/stderr to the serial console**, nothing else. No
  syslog, no log files, no remote shipping.
- **HTTPS just works.** Every image ships the Mozilla CA bundle at the
  standard system path; see "HTTPS calls and the CA bundle" below.

## Supervision

`gosd-init` runs as PID 1. After early setup (mounting `/dev`, `/proc`,
`/sys`, `/run`, setting the hostname, mounting the boot partition) it
starts `/app` and supervises it for the rest of the device's life:

- If `/app` exits, `gosd-init` restarts it automatically, with a capped
  exponential backoff so a fast crash loop doesn't spin the CPU or flood
  the serial console; running stably for a while resets the backoff. See
  `cmd/gosd-init/internal/boot/supervisor.go` and `backoff.go` for the
  exact policy.
- There's no restart limit — your app is expected to run forever, or to
  be restarted forever if it can't.
- `/app`'s stdout and stderr are connected directly to the serial console
  (see "Logging" below); `gosd-init`'s own log lines go to the same
  console, each prefixed `[gosd] ` so the two are easy to tell apart.
- If something in early boot fails fatally (e.g. the boot partition never
  mounts), `gosd-init` logs the error, syncs, and reboots the device —
  your app is never left running with a boot sequence half-completed.
- A bug in `gosd-init` itself gets the same treatment: every long-running
  goroutine it starts is wrapped so a panic prints its stack to the console
  and reboots, and every board boots with `panic=10` on the kernel command
  line, so even a panic gosd-init can't catch reboots after 10s instead of
  leaving an unattended device hung until someone unplugs it.

## Environment variables

`gosd-init` sets three environment variables before starting `/app` (see
`cmd/gosd-init/internal/boot/sequence.go`):

| Variable | Value |
|---|---|
| `GOSD_BOARD` | The board ID the image was built for (e.g. `pi-zero-2w`), from `config.json` — overridable at boot via the `gosd.board=` kernel command-line parameter. |
| `GOSD_HOSTNAME` | The hostname `gosd-init` just applied via `sethostname(2)`. |
| `GOSD_DATA_FLUSH` | `1` if `/data` (and any `emmc`/`disk` vfat mount your app makes) uses the vfat `flush` mount option, `0` otherwise — `gosd build --data-flush`'s baked default, overridden by the card's `config/data_flush` file when it's set to anything. The `emmc`/`disk` packages read this themselves (see "Storage" below); most apps never need to. |

There's deliberately no `GOSD_DATA`: persistent storage always lives at
the fixed path `/data` (see "Storage" below), so there's nothing to
communicate — write there directly. There's likewise no `GOSD_IP` or
similar: networking comes up asynchronously after `/app` has already
started (see below), so no address is known at the time `/app` launches.
If your app needs its own address, discover it at runtime with
`net.InterfaceAddrs()` / `net.Interfaces()` rather than expecting it to
be handed to you.

## App environment variables

Beyond the `GOSD_*` vars above, your app can also receive whatever plain
key/value settings its deployment needs — read them the normal way, with
`os.Getenv`. There's nothing GoSD-specific about consuming them; the
GoSD-specific part is where the values come from. `examples/hello` reads
an optional `GREETING` var this way (see its `main.go`).

Each one is its own file under `config/env/` in [the config
tree](config.md) — a file named `GREETING` holding `Hello!` sets the
`GREETING` variable to `Hello!`. `gosd-init` merges two sources **per key**
(not as a whole-map replace) before starting `/app` — see `mergeUserEnv` in
`cmd/gosd-init/internal/boot/sequence.go`:

| Source | Wins per key? | Where it lives |
|---|---|---|
| `config/env/<NAME>` | Yes | Hand-editable fallback on the boot partition (see "Provisioning" below). |
| Baked defaults | No | Recorded in `config.json` inside the image, from your app's own `--config-dir` overlay. |

Precedence is evaluated per key: if the card sets `LOG_LEVEL` but not
`API_URL`, and a baked default set both, your app gets the card's
`LOG_LEVEL` alongside the baked `API_URL` — not one source or the other
in its entirety.

To give each device *different* values without building it a different
image — a per-user API token spliced in as the image downloads, say — see
[injecting settings](image-injection.md#injecting-settings).

Your app's environment is otherwise a clean slate: it gets exactly the
`GOSD_*` vars above plus this merged user env, not a copy of
`gosd-init`'s own environment (`os.Environ()`).

**Reserved names.** Keys in `gosd-init`'s own `GOSD_*` namespace
(`GOSD_BOARD`, `GOSD_HOSTNAME`, `GOSD_DATA_FLUSH`, and any future `GOSD_*`
var) can never be set this way. `gosd build` refuses a `config/env/GOSD_*`
file outright, with an actionable error, before it ever reaches an image. A
`GOSD_*` file hand-written into a card's `config/env/` is logged and
ignored at boot instead — your app always gets `gosd-init`'s real value for
those, never whatever a card tried to override them with.

**Missing or empty is fine.** No baked defaults and nothing set under
`config/env/` on the card is a normal, unremarkable boot: your app just
gets none of these vars (plus the `GOSD_*` ones above), and nothing errors
either way.

**Security note.** Like the WiFi passphrase stored in the same tree, an
`env/` value sits in plaintext on the boot FAT partition — anyone with
physical access to the card, or who mounts the image, can read them.
There's no encryption today; don't put anything there you wouldn't want
exposed to whoever holds the card.

## Networking comes up after your app does

`gosd-init` never blocks `/app`'s startup on networking. Network bring-up
(link up, DHCP, DNS, and reacting to a cable being pulled/replugged) runs
in its own goroutine, started just before `/app` is launched, not before.

Practical implications for your app:

- **Never assume connectivity at startup.** Retry any network operation
  (dialing out, listening for inbound connections that depend on
  routing, etc.) rather than treating a failure at process start as
  fatal.
- **`/run/gosd/network-up`** is an empty marker file `gosd-init` creates
  once an interface has a usable address, and removes if that link later
  goes down (see `cmd/gosd-init/internal/netup/resolvconf.go`). Polling
  for its existence is a reasonable way to gate work that specifically
  needs an address, but plain retry-on-failure works too and doesn't
  need you to poll the filesystem.
- **DNS** is written to `/etc/resolv.conf` from the DHCP lease once one
  is obtained; it's simply absent before then.
- **`localhost` resolves via the shipped `/etc/hosts`**, no DNS or network
  needed — every image ships one baked into the initramfs (see
  `internal/hostsfile`). Your device's own hostname resolves to `127.0.1.1`
  too, once `gosd-init` has settled on it during boot.

`gosd-init` brings up wired Ethernet (interfaces matching `eth*`, `end*`,
`enp*` — see `cmd/gosd-init/internal/netup/netup.go`) and, if the board
has WiFi hardware, associates to a single WPA2-PSK or open network (see
`cmd/gosd-init/internal/wifiup`) using the same DHCP/DNS bring-up either
way. WPA3/EAP networks are out of scope through v0.x; `gosd-init` logs
clearly and skips WiFi bring-up rather than attempting to join one.

The network to join comes from whichever of these sources names one:

1. **`config/wifi/ssid` and `config/wifi/passphrase`** — the hand-editable
   fallback on the boot partition (see "Provisioning" below). A Raspberry Pi
   Imager wizard's answer reaches this tier too, as an ordinary setting
   written into the tree once its cloud-init seed is consumed — there's no
   separate, competing tier for it.
2. **`config.json`**, baked at build time — used only when the card names
   no network at all.

### HTTPS calls and the CA bundle

An outbound HTTPS request from `/app` just works: every image ships the
Mozilla CA bundle (curl.se's dated `cacert.pem` snapshot, pinned in
`internal/cacerts`) at `/etc/ssl/certs/ca-certificates.crt`, Go's default
root-certificate path on Linux, so `crypto/x509` finds it with no setup —
no import, no build-time step, nothing your app needs to do. The bundle is
baked in at `gosd build`/`gosd run` time and updates with each `gosd`
release as the pin is bumped (bean gosd-kzgq).

An app can still pin its own roots at build time instead, via a blank
import:

```go
import _ "golang.org/x/crypto/x509roots/fallback"
```

This still works (the image's own bundle just makes it unnecessary for
most apps) and is the one way to control exactly which roots ship,
independent of `gosd`'s own release cadence. See `examples/sattrack/main.go`
for the pattern in production use, calling a TLE API over HTTPS.

This is a separate concern from the clock (below): a valid CA bundle
doesn't help if the clock still reads 1970, since certificate validity
periods won't check out either — see "Clock" for that gotcha.

### Ingress: reaching your app from the internet (`--ingress`)

An image built with `gosd build --ingress cloudflared` or
`--ingress tailscale-funnel`, plus a matching `config/ingress/<agent>/`
group filled in on the boot partition, gets a public URL: `gosd-init` supervises a
baked-in binary that carries traffic for one hostname to one local port on
your app — no port forwarding, no public IP address, and no app code
required. Cloudflare Tunnel needs a Cloudflare account and only runs on
arm64 boards; Tailscale Funnel needs a Tailscale account, runs on every
board (including `pi-zero-w`), and keeps its tailnet identity on `/data` so
it survives a reflash with no re-authentication.

**[docs/ingress.md](ingress.md) is the full guide** for both agents —
creating the tunnel or registering the tailnet node, what each one needs
from your Cloudflare/Tailscale account and can't set up for itself, what
files or state actually exist and where, the clock/TLS startup window, the
pinned-version or compiled-per-arch update story, and what survives a
reflash.

## Provisioning: hostname and WiFi from Raspberry Pi Imager

Beyond `config.json` baked in at build time and [the config
tree](config.md) hand-edited on the card, `gosd-init` also reads whatever
Raspberry Pi Imager's customization wizard wrote to the boot partition —
cloud-init's `user-data` (hostname) and `network-config` (WiFi access
points) — see `internal/provision` and `docs/provisioning-formats.md` for
the full field mapping. This is the flagship end-user flashing path:
publish a custom-repository catalog entry for your image
(`init_format: "cloudinit"`) and Imager's full WiFi/hostname wizard
becomes available for anyone flashing it.

The wizard's answers don't compete with the card at runtime — they become
the card. `gosd-init` deletes the cloud-init files, durably, and only then
writes what they asked for into `config/hostname` and `config/wifi/*` (see
[how the wizard's answers land in the tree](config.md#how-the-imager-wizards-answers-land-here)),
so from that point on a wizard-provisioned value is indistinguishable from
one somebody typed in by hand — including surviving a reflash the same way
(see "Keeping settings across a reflash" below).

Practical notes:

- `gosd-init` only ever consumes hostname and WiFi from these files —
  everything else the wizard can configure (users, SSH keys, locale,
  passwordless sudo, ...) is RPi-OS-specific and silently ignored, since
  `gosd-init` has no shell or user accounts to apply them to.
- `firstrun.sh` (Imager's older, non-cloud-init mechanism) is **never**
  parsed or executed — if one is found on the boot partition, `gosd-init`
  logs a line pointing you at the config tree instead.
- A malformed or partially-written provisioning file is logged and
  skipped; it never blocks boot.

## Clock: starts at 1970 until SNTP syncs

No GoSD board has a battery-backed real-time clock. On boot, the system
clock starts at the Unix epoch and only becomes correct once SNTP sync
completes.

`gosd-init` syncs the clock itself (`cmd/gosd-init/internal/timesync`) —
your app doesn't need to do anything to make this happen:

- Once `/run/gosd/network-up` appears, `gosd-init` queries NTP (retrying
  with backoff until the first success), sets the clock via
  `settimeofday`, and re-syncs hourly afterwards for the life of the
  device.
- The server list comes from `config.json`'s optional `ntpServers` field
  (baked in by `gosd build`); when it's absent — including every image
  built before this field existed — it defaults to `pool.ntp.org`.
- **`/run/gosd/time-synced`** is an empty marker file `gosd-init` creates
  on the first successful sync. Gate anything that checks certificate
  validity periods (TLS handshakes, `crypto/x509` verification) on this
  file existing — attempting those before the clock is correct fails,
  because the clock may still read 1970. Polling for the marker or
  simply retrying TLS-dependent operations on failure both work; either
  way, don't treat an early failure as permanent, since the clock does
  become correct within moments of the network coming up.
- SNTP is unauthenticated UDP, so `gosd-init` doesn't trust any single
  result outright: a reply reporting a time before the image was built
  is refused and logged (the build timestamp is baked into `config.json`
  alongside `ntpServers`), and once the clock is synced, a later hourly
  resync that would step it by an implausibly large amount is also
  refused and logged rather than applied — unless an immediately
  following resync reports a consistent value too, which is what lets a
  device that was genuinely powered off for a long time still catch up
  instead of being stuck refusing forever.

## Storage

Three tiers, in increasing order of durability: a RAM-backed root
filesystem that's wiped every reboot, a read-only `/boot`, and an
opt-in, persistent `/data`.

### Root filesystem: RAM, wiped every reboot

GoSD's boot sequence never leaves the initramfs: there's no `pivot_root`
or `switch_root` to a separate root filesystem. The root filesystem your
app runs on is Linux's initramfs `rootfs` — a RAM-backed, writable
filesystem — so:

- Anything your app writes outside `/data` and `/boot` is writable at
  runtime, but **lives in RAM and is gone on reboot or power loss.** For
  durable writes, use `/data` (below).
- `/boot` — the boot FAT partition containing the kernel, initramfs, and
  boot configuration — is mounted **read-only**. Don't expect to write to
  it from your app. Its volume label is per-app: `<prefix>-boot`, where
  `<prefix>` defaults to the app's own name (truncated to 6 bytes) and can
  be overridden with `gosd build --label-prefix` — so an app called
  `hello` shows up as a drive named `hello-boot` when the card is plugged
  into a computer.
- Because the rootfs is RAM-resident, be mindful of memory: GoSD targets
  small, memory-constrained devices (see `COMPATIBILITY.md`), and
  anything you write to the rootfs is really consuming RAM.

### Persistent storage: `/data`

Images are built with a second partition, labelled `<prefix>-data` (the
same per-app prefix as the boot partition, e.g. `hello-data`), sized by
`gosd build --data-size` and formatted by `gosd build --data-filesystem`
(`fat32` or `ext4`; default `fat32`). It's opt-in: the default size is `0`
(no partition at all), so pass a size (e.g. `--data-size=1GiB`) to get one.
`gosd-init` mounts it read-write at the fixed path `/data`. Data written
there survives reboots and power cycles. There's no environment variable
to consult — `/data` is always the path; just write to it.

**Don't rename the volumes.** The data label is what the device compares an
established data partition against on every boot, so relabelling it from a
computer looks exactly like a corrupted partition: the device halts rather
than risk destroying app data (see the halt described below). Renaming the
boot volume is harmless, but tell your users not to rename either — they
can't tell the two apart from the desktop.

#### Choosing a filesystem: FAT32 or ext4

By default `/data` is **FAT32** — readable and repairable from any
computer's SD card reader, with the limits described below. `gosd build
--data-filesystem=ext4` opts into a journaled **ext4** data partition
instead: the journal buys metadata crash-consistency and mount-time
replay, so a data partition interrupted mid-write to its own metadata (a
directory entry, an inode) recovers cleanly at the next mount instead of
needing an fsck. That is **not** the same guarantee as data durability —
the four-step fsync sequence described under "Making a write durable"
below remains the app-facing contract for durable *file content* either
way, journal or not.

The cost: an ext4 data partition can't be read or repaired from a macOS or
Windows host the way a FAT32 one can (it needs Linux-side tooling). It
also depends on the board's stock kernel building ext4 support in:
`gosd build --data-filesystem=ext4` refuses, naming the alternative, for
any selected board whose kernel doesn't — no board GoSD currently ships
falls into that group; see `COMPATIBILITY.md`'s ext4 data partition row
for the per-board detail.

Like `--boot-size` and the label prefix, the chosen filesystem is part of
the app's on-card layout ABI — see [the upgrade path design](design/upgrade-path.md) for
the argument in full — so changing it between releases erases and
re-establishes the data partition on the next upgrade: a release-notes-level
breaking change, not corruption.

`--data-size=expand` is the fill-the-card variant: the image ships with
no data partition at all (staying 272MiB to download and flash), and the
device creates one itself, exactly once, on its first boot — an MBR
entry covering the rest of the card, formatted per `--data-filesystem`
(FAT32 by default, or ext4), labelled with the app's configured data label
(`<prefix>-data`), and mounted at `/data` like any other data partition
from then on. Points specific to expand:

- **Only the disk the device actually booted from is ever touched** —
  the same verified device the boot partition mount used — and only when
  its partition table is exactly the one a GoSD image ships (boot
  partition in place, no partition 2). Anything else is left alone,
  loudly.
- **First boot takes a few extra seconds** while the partition is
  created and formatted; the serial console narrates it. Every later
  boot finds the partition present and does nothing.
- **Power loss during first boot is safe**: the partition-table entry is
  written last, as a commit record, only once the formatted filesystem
  is durable on the card — so a power cut anywhere mid-creation leaves
  no entry, and the next boot simply redoes the whole thing from
  scratch.
- **An established data partition is never "repaired" away.** If a later
  boot finds the partition entry in place but the data partition's
  filesystem gone (a failing card, say), the device writes what happened to
  `LAST_FATAL_ERROR.md` at the root of the boot partition — readable
  on any computer the card is plugged into — and **halts**, so whatever
  data survives can still be salvaged. To recover: save what you need
  from the partition, then either reformat it with the app's data label in
  the same filesystem the image was built with (FAT32 by default, or ext4)
  or delete partition 2 entirely and let the next boot recreate it, empty.
  That second option is specific to `--data-size=expand`, the only mode
  where the device creates the partition itself; a fixed-size image ships
  partition 2 in the image, so there the equivalent is deleting partition 2
  and flashing the image again. The report the device writes says which of
  the two applies to it — see [the crash report guide](crash-reports.md).
- **A card with no meaningful room** (less than ~64MiB beyond the image
  — including `gosd run`'s qemu disk, which is exactly image-sized) gets
  no partition, and `/data` behaves like a `--data-size=0` image:
  read-only, writes fail with `EROFS`.
- **A FAT32 partition is capped at 256GiB** for now (a FAT32-formatter
  limitation — see [How big the data partition can
  be](#how-big-the-data-partition-can-be)); a bigger card's remainder stays
  unused, with a log line saying so. An ext4 partition
  (`--data-filesystem=ext4`) has no such ceiling and always fills the
  whole remaining card.
- **A reflash re-adopts the existing partition, contents intact — this is
  why `--data-size=expand` is the recommended mode for updatable
  deployments.** Flashing an image rewrites the whole card's MBR, which
  drops partition 2's entry, but an `expand` image ships no data partition
  at all, so the flash never touches the bytes beyond the boot partition.
  On the next first boot, the device looks at what's actually sitting
  there: if it's a filesystem matching what the image was built to expect
  (FAT32 by default, or ext4 with `--data-filesystem=ext4`) labelled with
  this image's configured data label (`<prefix>-data`) and carrying a
  hidden completion marker (`gosd-data-established`, at the partition's
  root — see the marker note below), it's adopted rather than reformatted,
  and the MBR entry is simply rewritten to record it. The marker matters
  because a matching label alone isn't proof of a finished format — see
  below — so the device only ever adopts a partition it can prove an
  earlier boot completed; anything else (blank space, a foreign
  filesystem, a differently-labelled data partition, or the debris of a
  first-boot format that never finished) is formatted fresh, exactly as a
  first boot always was.
- **Changing `--boot-size` between releases breaks the adoption above.**
  The boot volume's size is baked into the image and fixes where the data
  partition starts; a later release that changes it (either direction) —
  see `docs/publishing.md`'s note on `--boot-size` — means the device
  can't recognize what's on the card as its own `/data`, and the next
  reflash wipes it cleanly instead of adopting it. That's a
  release-notes-level breaking change for that app, not corruption.
- **Changing `--data-filesystem` between releases breaks the adoption
  above too, the same way.** A FAT32 data partition isn't an ext4 one and
  vice versa, so a release that switches filesystems can't recognize
  what's already on the card as its own `/data` either; the next reflash
  reformats it fresh to the newly requested filesystem instead of
  adopting it — another release-notes-level breaking change, not
  corruption (see [the upgrade path design](design/upgrade-path.md)).
- **Changing `--label-prefix` — or renaming the app, since the prefix
  defaults to the app's own name — between releases breaks the adoption
  above too.** The data label is part of the app's on-card ABI exactly
  like `--boot-size` and `--data-filesystem`: a reflash-upgrade that finds
  a data partition labelled with the old prefix treats it as debris and
  cleanly reformats it, rather than halting — the boot partition is
  unaffected either way. This is a **clean break with no migration**:
  cards flashed by a pre-`--label-prefix` release carry `GOSD-DATA`, and
  the first reflash-upgrade with a rebuilt image reformats that data
  partition. It also means a cross-app reflash no longer silently
  inherits the previous app's data the way every app sharing the fixed
  `GOSD-DATA` label once did — unless the two apps happen to share the
  same 6-byte prefix, which re-adopts across apps exactly as today's
  universal label did, just narrower.
- **A fixed-size `--data-size` partition is still wiped by every
  reflash.** It's formatted and embedded inside the `.img` file itself,
  so flashing any version overwrites the data region directly — there's
  nothing to adopt, on any release.

Rules of engagement:

- **When there's no partition, `/data` is read-only.** If the image was
  built with the default `--data-size=0` (or no `--data-size` at all),
  or the card's data partition can't be mounted (a bad card, say),
  `gosd-init` mounts an empty **read-only** filesystem at `/data`
  instead — boot still proceeds normally. A write then fails
  immediately with `EROFS` rather than silently landing in RAM and
  vanishing on the next reboot. This is deliberate: it turns "I thought
  I had persistence" into a loud error at the write, not silent data
  loss. A well-behaved app treats an `EROFS` write to `/data` as "no
  persistence available this boot" rather than a fatal error.
- **By default it's FAT32, with FAT32's limits.** No unix permissions, no
  ownership, no symlinks or hard links, 4GiB max file size, coarse (2s)
  mtime granularity. Don't design around any of those existing — or build
  with `--data-filesystem=ext4` for full unix semantics and no 4GiB
  ceiling, at the readability/compatibility cost described above under
  "Choosing a filesystem".
- **Neither filesystem is power-loss-robust for file *content*.** FAT has
  no journal at all: a power cut mid-write can corrupt the file being
  written (and, less commonly, the filesystem) whether or not the `flush`
  mount option below is on. An ext4 data partition's journal narrows this to
  *metadata* only — see "Choosing a filesystem" above — the file content
  you're actually writing gets no more protection from ext4's journal
  than it does from FAT's total absence of one. Never rewrite your only
  copy of something in place — write durable state the boring, robust
  way, described in full under "Making a write durable" below, regardless
  of filesystem.
- **The `flush` mount option is opt-in, off by default, and vfat-only.**
  `gosd build --data-flush` (default `false`) bakes in whether a FAT32
  `/data` — and any `emmc`/`disk` vfat mount your app makes — uses vfat's
  `flush` option, which pushes a file's data and metadata to the card
  promptly on `close(2)`. The default leaves it off: normal Linux
  writeback (~30s `dirty_expire_centisecs`) is fast, and `flush` was
  never enough for durability on its own anyway — a `rename` involves no
  `close`, so it doesn't touch the gap "Making a write durable" below
  closes. Turning it on trades write throughput for prompter (but still
  not durable by itself) writeback; setting the card's `config/data_flush`
  file to anything overrides the baked default per device, empty meaning
  "use the baked value" (see the `GOSD_DATA_FLUSH` env var
  above, which is how `emmc`/`disk` — mounting from your app's own
  process — learn the effective setting). `flush` has no ext4 equivalent
  and no effect on an ext4 data partition — `gosd build` refuses
  `--data-filesystem=ext4` combined with `--data-flush` outright, rather
  than silently ignoring the flag.
- **`/data/.gosd-data`** is an empty marker file `gosd-init` creates the
  first time the partition mounts; leave it alone, and don't be
  surprised by it when listing `/data`. An `expand` image's partition
  root also carries `gosd-data-established` (not a dotfile — deliberately,
  see the reflash bullet above) once its first-boot format completes, and
  `/data/.gosd/` is reserved for `gosd-init`'s own bookkeeping — the config
  store (below) and, on an image built with `--ingress tailscale-funnel`,
  the shim's tsnet state under `/data/.gosd/tailscale` (see
  [docs/ingress.md](ingress.md)). Leave all three alone; none of them
  is meant for your app to read, and an app deleting one is never treated
  as corruption.
- **Reflashing wipes `/data` for a fixed-size `--data-size` image, every
  time.** It's embedded in the `.img` file itself, so flashing any later
  version overwrites the data region directly. Built with
  `--data-size=expand` instead, `/data` survives a same-`--boot-size`
  reflash (see above) — this is the one durability difference between the
  two modes, and the reason to prefer `expand` for anything you expect to
  update. There is no over-the-network alternative to reflashing, now or
  later: updating a fielded card means flashing it again, permanently (JP,
  2026-08-21). An app-slot scheme that would have changed that was designed
  and then declined; it is [kept as a record of a road not
  taken](design/ab-updates.md), and the reasoning is at the top of it.

### Keeping settings across a reflash: the config store

Reflashing rewrites the whole of the boot partition, [the config
tree](config.md) included — so without more than the above, every
hand-edited `env/` value and every wizard-provided hostname/WiFi credential
would be replaced by the new image's baked defaults on every upgrade. On a
`--data-size=expand` image (or any image with a non-zero `--data-size`),
`gosd-init` closes that gap the same way it protects `/data` itself: every
boot, it records — on `/data`, under `/data/.gosd/config` — every setting
whose card file differs, byte for byte, from the value this image shipped
it with. No data partition means nothing is kept, and nothing self-heals —
one more reason to build with `expand` for anything you expect to update.

Restoring only ever runs on the first boot under a *different* image than
the one the kept copy was last reconciled against (a genuine reflash to a
new build), detected the same way as everywhere else: the image's own
identity, a content-derived digest `gosd build` bakes into `config.json`,
logged at boot as `image identity: <short digest>`. Flashing the *same*
image back over a card doesn't trigger a restore at all — every setting
already reads exactly as that image ships it, which is indistinguishable
from someone having reverted every value by hand.

Per setting, on that first boot:

1. **Whatever the freshly flashed card already says, if it differs from
   what this image shipped, wins outright.** An injected value, or a
   hand-edit (including one an Imager wizard just wrote in) already sitting
   on the new card, is the freshest statement of intent there is.
2. **Otherwise, the kept value is written back onto the card** — restoring
   what the previous image's card held. If this image's own default for
   that setting differs and isn't empty, it also appears beside the
   restored value as `<name>.new`, purely informational (the device never
   reads it back). A kept value the new image has no file for at all is
   handed back as `<name>.unused` and then forgotten — one reflash to
   retrieve it — except under `config/env/`, a customer's own namespace,
   which is never treated as an orphan.
3. **Anything neither kept nor present on the freshly flashed card** is
   whatever this image's own defaults name, exactly as on a first flash.

The practical effect: an operator who reflashes a device the same way they
flashed it the first time — including skipping the Imager wizard entirely,
via "Use custom image" — gets their hostname and WiFi back and rejoins the
network on its own, any hand-edited `env/` value survives too, restored
directly into the same file it was set in, and a configured [Cloudflare
Tunnel](ingress.md) resumes the same way — with no credentials file to
lose, since the tunnel token round-trips through `/data` exactly like a
WiFi passphrase does. A configured [Tailscale Funnel](ingress.md) does even
better: its tailnet identity lives on `/data` independently of the config
store, so it reconnects under the same public URL with no re-authentication
at all — the store here only carries back the remaining
`hostname`/`port`/`funnel_port` (and the auth key, if one is still sitting
on the card).

See [the config tree's guide to this
mechanism](config.md#keeping-your-settings-across-a-reflash) for the full
per-setting rules, including how to deliberately put a value back to its
default and how to unset a kept `env/` value.

What does **not** come back: anything outside the config tree itself — the
Imager wizard's other, RPi-OS-specific settings were never applied in the
first place (see
"Provisioning" above) — nor the schema or contents of your app's own
`/data` files across versions, which remains the app's
concern like any other update. Every step here is best-effort: a store
entry that's missing, torn, or unreadable, or a read-only/absent `/data`,
is logged and skipped — never a boot failure.

### Making a write durable

Write to a temporary name, then rename it over the real one — with two
extra syncs after the rename that FAT specifically needs:

```go
f, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
// ... write, handle errors ...
f.Sync()                  // 1. the new contents are on the card
os.Rename(path+".tmp", path) // 2. the name flips, atomically for readers
f.Sync()                  // 3. the renamed file's directory entry
f.Close()
d, _ := os.Open(filepath.Dir(path)) // 4. the directory's own entries
d.Sync()
d.Close()
```

What each half buys you:

- **Steps 1-2 give crash *consistency*.** A reader — this boot, or after any
  power cut — sees either the whole old version or the whole new one, never a
  torn mix. If that's all you need (the write is a cache, or a fresh one
  happens every few seconds anyway), stop there.
- **Steps 3-4 give *durability*: the new contents survive a power cut that
  happens immediately.** Without them there's a window of up to ~30s in
  which the file silently reverts to its old contents, because a `rename`
  only dirties directory blocks, and dirty FAT directory blocks wait for
  the kernel's normal writeback expiry (`dirty_expire_centisecs`, 30s) to be
  written. The `flush` mount option (opt-in, see "Persistent storage" above)
  doesn't cover this even when it's on: it flushes a file's data and
  metadata on `close(2)`, and a rename involves no close. This bit us for
  real, back when `flush` was mounted unconditionally —
  `examples/hello`'s boot counter never survived a power cut less than
  ~30s after boot (bean `gosd-0nk4`), which is exactly why steps 3-4 exist
  regardless of the mount option.
- **Step 3 is not optional, and is the surprising one.** The rename writes
  the new directory entry with a zero start cluster and size; only the
  *file's* own `fsync` (or, eventually, writeback) fills those in. `fsync`
  the directory alone and a power cut can leave the new name on the card as
  an **empty** file — worse than losing the rename. Sync the file first,
  then the directory.

`fsync` on a FAT file *and* on a FAT directory both end with a cache-flush
command to the card, so "durable" here means durable against the card's own
volatile write cache too, not just the kernel's.

The price is real but small: each durable write costs a few extra small
writes to the card. Don't do this in a tight loop on data you'd be happy to
lose — batch it, or accept steps 1-2 only.

GoSD deliberately leaves that choice to you. It would be possible to mount
`/data` with `dirsync`, making every directory operation synchronous so that
steps 3-4 were never needed — but on SD and eMMC a small synchronous write
means the card rewriting a whole erase block, so that would tax every write
by every app to protect the few that need durability, and it *still*
wouldn't remove the need to sync file data. So the mount stays as it is and
the decision is the app's.

For a worked example, `examples/hello` persists a boot counter to `/data`
with exactly this sequence (`writeFileDurably` in its `main.go`), and
reports "no-data-partition" when the write comes back `EROFS`. CI's
qemu data-partition job kills the machine seconds after that write and
asserts the counter still comes back incremented on the next boot.

### How big the data partition can be

This section is about the **default, FAT32** data partition; an ext4
one (`--data-filesystem=ext4`) works the other way round — see the floor
described at the end of this section.

**256.06 GiB (274,940,836,864 bytes) is the FAT32 ceiling**, and `gosd build`
enforces it: `--data-size=400GiB` is refused at the flag, before anything is
compiled or written, naming the maximum. `--data-size=expand` caps itself at a
round 256GiB on a larger card and logs how much of the card it left unused.

Below that ceiling, any size of at least one sector (512 bytes) works —
though the volume that gets written may be up to two clusters (at most
32.5KiB) smaller than the size you asked for. The formatter trims to the
largest size it can lay a self-consistent FAT out for; without that,
roughly one whole-GiB size in nine — 16, 32, 64, 128 and 256GiB among them
— produces a volume that macOS First Aid and Windows chkdsk report as
damaged (bean `gosd-e3e3`). A size below one sector is refused at the flag
too, for the same reason as the ceiling: it can never back a real
partition, and is almost always a missing unit suffix (`--data-size=100`
meaning 100 bytes rather than 100MiB) rather than an intentional choice.

The reason for the ceiling is GoSD's pure-Go FAT32 formatter, which counts the
sectors in each file allocation table in 16 bits: past that size the volume
would be laid out
with FATs far too small to address its own clusters, and — worse than an
error — written out and mounted as if it were fine. Refusing is deliberate. It
is not a limit of FAT32 itself, and lifting it is tracked in beans `gosd-8kdm`
and `gosd-mt53`; when it goes, it goes for `--data-size`, for `expand`, and for
the `disk`/`emmc` packages at once.

If your app needs more storage than that, the answer today is an attached disk
rather than a bigger `/data`: the [`disk` package](#attached-disk-storage-disk-package)
formats an SSD or USB drive as **ext4** by default, or **exFAT** on request —
neither has this ceiling or FAT32's 4GiB-per-file one. `/data` remains the
place for state; bulk media belongs on the drive holding it.

An ext4 data partition (`--data-filesystem=ext4`) has a **floor**, not a
ceiling: GoSD writes a fixed, checked-in 512MiB golden ext4 image and grows
it online to the partition's real size on first boot, so `--data-size` must
be at least that same 512MiB — anything smaller is refused at the flag,
before anything is compiled, naming the minimum. `--data-size=expand`
always clears it, since even the smallest card GoSD targets leaves far more
than 512MiB free beyond the boot partition. There's no ext4-side ceiling
equivalent to FAT32's 256GiB one — it grows via the kernel's own online
resize, not GoSD's pure-Go FAT32 writer.

## Onboard eMMC storage (Rockchip boards)

Some Rockchip boards also have an eMMC in addition to the microSD card
they boot from — the Pi boards have no such thing. On the Radxa Zero 3E
and NanoPi Zero2 it's soldered on; on the Radxa ROCK 4SE it's an optional
plug-in module, so `emmc.FormatAndMount` reports "no eMMC" unless one is
fitted (see `COMPATIBILITY.md`'s onboard-eMMC row for per-board status).
The public `emmc` package lets your app format and mount whichever the
board has. (For any *attached* mass storage — an NVMe SSD, a USB drive —
see "Attached disk storage" below, which has the same shape.)

```go
if err := <-emmc.FormatAndMount("APPDATA", "/storage", false); err != nil {
	log.Printf("no persistent storage: %v", err)
}
```

`FormatAndMount` returns immediately; the formatting/mounting work runs
in the background, and the returned channel receives exactly one value —
`nil` once the mountpoint is ready, or an error — before closing. Block
on it only once your app actually needs the storage.

- **The eMMC is discovered automatically**, distinguishing it from the
  microSD card the board is currently running from — the boot device is
  never a format target, so there's no risk of an app wiping the card
  it's running on.
- **Formatting is idempotent, keyed on the label AND filesystem you pass.**
  An eMMC already carrying a volume labelled `label` *and* formatted as the
  requested filesystem is only mounted, never reformatted (nor re-grown,
  for ext4 — see below) — this is how a second run (or every run after the
  first) avoids wiping its own data. A blank eMMC (no filesystem at all) is
  always formatted, even with `destructive` set to `false`.
- **`destructive` guards everything else**, including a label match against
  a *different* filesystem than requested — e.g. an eMMC an earlier build
  of your app formatted before ext4 became the default, or a previous
  `emmc.Options.Filesystem` choice. `false` makes `FormatAndMount` refuse
  and return an error naming both filesystems rather than touch it; `true`
  wipes and reformats it as what was asked for.
- **ext4 is the default** (`Options.Filesystem`'s zero value, `emmc.EXT4` —
  a deliberate breaking change from `emmc`'s earlier FAT32-only default,
  bean `gosd-9sc4`, mirroring `disk`'s own flip token-for-token): journaled
  and crash-safe, unlike FAT32/exFAT, which is exactly what matters for an
  internal, non-removable eMMC. FAT32 and exFAT remain available as
  explicit `emmc.Options{Filesystem: emmc.FAT32}` (or `emmc.ExFAT`)
  choices. Everything under ["ext4 by default, or FAT32/exFAT for
  removable media"](#ext4-by-default-or-fat32exfat-for-removable-media)
  below — the golden-image format/grow/adopt mechanism, the
  `CONFIG_EXT4_FS`/`CONFIG_EXFAT_FS` kernel preflight and
  `ErrUnsupportedFS`, the FAT-family ceilings, and the 16-byte-vs-11-byte
  label limit — applies to `emmc` identically; swap in
  `emmc.FormatAndMountWith`/`emmc.Options` for `disk`'s equivalents. Unlike
  `disk`, there's no `emmc.Devices`/`FormatAndMountDevice`: `emmc` always
  addresses the board's one onboard eMMC.
- **It's a whole-device filesystem** — the mount source is the raw
  `/dev/mmcblkN` device, not a partition on it — with the same limits as
  `/data`: no unix permissions, ownership, symlinks, or hard links. Only
  ext4's journal buys crash *consistency*, and only for its own metadata —
  write durable state with the same four-step sequence described under
  "Making a write durable" above regardless of which filesystem is in
  play. A FAT32 mount also honors the same `GOSD_DATA_FLUSH` setting as
  `/data` (see "Persistent storage" above); ext4 and exFAT never took the
  `flush` mount option and are unaffected either way.
- **On a board with no onboard eMMC** (the Pi boards, a Rockchip board
  whose only eMMC turns out to be the boot device, or an unfitted ROCK
  4SE module), `FormatAndMount`'s channel yields `emmc.ErrNoEMMC` — check
  for it with `errors.Is` and treat it as "no eMMC here" rather than a
  fatal error, the way `examples/emmcstorage` does.

`examples/emmcstorage` is the worked example: it formats and mounts the
eMMC at `/storage`, degrades gracefully (logs and exits cleanly) when
`ErrNoEMMC` comes back, and otherwise writes a small file and reads it
back to demonstrate persistence.

## Attached disk storage (`disk` package)

The public `disk` package is the general-purpose sibling of `emmc`:
where `emmc` addresses one specific device (a board's onboard eMMC),
`disk` takes whatever mass storage it finds attached — an M.2 NVMe SSD,
a USB drive, an SD card in a USB reader. The call is the same shape:

```go
res := <-disk.FormatAndMount("APPDATA", "/storage", false)
if res.Err != nil {
	log.Printf("no bulk storage: %v", res.Err)
}
```

Everything `emmc` guarantees, `disk` guarantees identically: it returns
immediately and the channel delivers exactly one `Result` before
closing; formatting is idempotent and keyed on the label, so a disk
already carrying a volume with your label *and* filesystem is only
mounted (not re-formatted, nor re-grown — see "ext4 by default" below);
a blank disk is always formatted; `destructive` gates everything else.

- **Discovery is an allowlist, and never picks the boot media.** Only
  `nvme*` (NVMe namespaces), `sd*` (SCSI/USB mass storage), `vd*`
  (virtio) and `mmcblk*` (SD/eMMC) can be chosen, and only if nothing is
  mounted from them. `/sys/block` is full of nodes that would be
  catastrophic or pointless to format — `loop*`, `ram*`, `zram*`, `zd*`,
  `dm-*`, `md*`, `sr*`, `nbd*`, `mtdblock*`, `ubiblock*`, and an eMMC's
  `boot0`/`boot1`/`rpmb` hardware partitions — and none of them is ever
  a candidate. A device reporting no medium (an empty card-reader slot)
  or write protection is skipped too.
- **When several disks qualify, the order is fixed**: NVMe, then
  USB/SCSI, then virtio, then MMC, and alphabetically within each class
  — so the choice never depends on which device the kernel happened to
  enumerate first. To pick deliberately, `disk.Devices()` lists the
  qualifying device nodes in that same order and
  `disk.FormatAndMountDevice("/dev/sda", …)` targets one. Naming a
  device explicitly still can't wipe a disk something is mounted from.
- **It's a whole-device filesystem** — the mount source is the raw
  `/dev/nvme0n1`, not a partition on it. That is deliberate: it's what lets
  `Result.BlockDevice` be handed straight to `gadget.MassStorage` to share
  the same volume over USB (`disk.Unmount` first — expose or mount, never
  both), and it avoids the privileged partition-table reread. A host plugging
  in sees a drive with no partition table, which Windows, macOS and Linux all
  mount happily.
- **When nothing suitable is attached**, `FormatAndMount`'s channel yields
  `disk.ErrNoDisk` — check for it with `errors.Is` and treat it as "no disk
  here" rather than a fatal error, exactly as `examples/emmcstorage` does for
  `ErrNoEMMC`.
- **A USB drive may not have appeared yet, and `Options.Wait` is how you
  wait for it.** Discovery looks once. That is right for an NVMe SSD or an
  eMMC, which sit on on-SoC buses and are enumerated well before your `main`
  runs, but a USB stick or enclosure has to have its hub port powered, be
  probed, be scanned, and report its medium ready — commonly a second or two
  after the USB host controller comes up, and longer through a hub or for a
  disk that spins. Reach `FormatAndMount` before that finishes and you get
  `ErrNoDisk` for a drive that is plugged in. `Options.Wait` keeps looking
  for that long:

  ```go
  res := <-disk.FormatAndMountWith("APPDATA", "/storage", disk.Options{
  	Wait: 10 * time.Second,
  })
  ```

  There is deliberately no default window — it would stall every app that
  treats `ErrNoDisk` as "carry on without one", and every board with nothing
  attached would pay it on each boot. A long `Wait` is also the honest way to
  say "use a drive whenever someone plugs one in", since true hotplug is the
  same problem seen from further away. Waiting only asks whether a disk has
  shown up; discovery, the boot-media exclusion and the in-use rule then all
  run exactly as they otherwise would.

`examples/diskstorage` is the worked example — it also doubles as CI's
`qemu-disk-ext4` job's test app, so it's exercised on every PR.

### ext4 by default, or FAT32/exFAT for removable media

Written in terms of `disk`, but this whole section applies identically to
`emmc` (see "Onboard eMMC storage" above) — read `emmc.FormatAndMountWith`/
`emmc.Options`/`emmc.EXT4`/`emmc.FAT32`/`emmc.ExFAT` for `disk`'s
equivalents throughout.

`FormatAndMount`'s zero value — and `FormatAndMountWith`'s
`Options{}` — formats **ext4**, not FAT32: a deliberate breaking change
from `disk`'s earlier default, called out in the release notes of the
version that shipped it (epic `gosd-lfu0`; `emmc` followed identically,
bean `gosd-9sc4`). Internal drives are what
`disk` is almost always used for, where host-OS readability doesn't
matter and crash-safety does, so that's what the zero value now buys
you. Formatting writes a pristine, checked-in golden ext4 image
(`internal/diskfmt/ext4golden`) straight to the device — no `mkfs.ext4`
binary, no pure-Go mke2fs — then grows it to the disk's actual size with
a single online `EXT4_IOC_RESIZE_FS` ioctl, exactly once, at first
establishment; a later mount of an already-established volume adopts it
rather than re-growing or reformatting it.

**Adoption is gated on a completion marker, on every filesystem.** GoSD
writes an empty `.gosd-established` file into the root of a volume only
once its format, the flush of that format to the medium, and (for ext4)
the one-time grow have *all* completed; a later boot adopts the volume
only if that marker is there. A matching label is not evidence: a format
interrupted by a power cut can leave one behind over a filesystem that
was never finished, and adopting that hands your app torn cluster
chains. Leave the file alone — it costs no space worth counting, and
removing it costs the volume its proof. A volume formatted by a release
older than this marker carries none, and is adopted anyway on the
evidence of the files already in it, so upgrading never reformats a card
that has your data on it. The
[full crash-ordering argument](../internal/blockmount/blockmount.go),
including what each interruption point leaves behind, is in
`internal/blockmount`'s package doc.

**The journal is not a substitute for the fsync pattern above.** ext4's
journal buys metadata crash-*consistency* (no half-written inodes or
directory entries after a power cut) and replays automatically at the
next mount — but, same as `/data`'s FAT32, it says nothing about
*your* file's data reaching the disk before a power cut. Durable writes
to an ext4-mounted `disk` volume use the exact four-step
write-sync-rename-sync sequence from "Making a write durable" above; the
journal changes what a crash can corrupt, not whether your own
unfsynced write survives one.

`CONFIG_EXT4_FS` is required in the board's kernel — `COMPATIBILITY.md`'s
"ext4 on attached disks" row says which boards have it, which today is
every board GoSD ships. Where it's missing, `disk` reports
`ErrUnsupportedFS` *before writing anything* — it reads
`/proc/filesystems` first — including when ext4 is only the silent
zero-value default:

```go
res := <-disk.FormatAndMount("APPDATA", "/storage", false)
if errors.Is(res.Err, disk.ErrUnsupportedFS) {
	// This board's kernel has no ext4 (e.g. a Pi); fall back explicitly.
	res = <-disk.FormatAndMountWith("APPDATA", "/storage", disk.Options{
		Filesystem: disk.FAT32,
	})
}
```

FAT32 and exFAT remain available as explicit `Options.Filesystem`
choices for the case ext4's default doesn't serve: **removable media
meant to be read on another host**, where FAT32's universal readability
— or exFAT's, past FAT32's two ceilings — is the point.

```go
res := <-disk.FormatAndMountWith("APPDATA", "/storage", disk.Options{
	Filesystem:  disk.ExFAT,
	Destructive: true,
})
```

Three things are worth knowing about the FAT-family options:

- **`FormatAndMountWith` writes FAT32** on request, with two hard
  ceilings: **no single file may exceed 4 GiB**, however large the disk,
  and **GoSD will only create a FAT32 volume up to 256 GiB** — point it
  at a 512 GB SSD and it refuses before touching the disk, naming the
  limit, because the FAT32 volume it would write past that size is
  corrupt (the pure-Go formatter counts the sectors in each file
  allocation table in 16 bits). **exFAT has neither ceiling**, and an
  exFAT disk that already carries your label is mounted, not
  reformatted — whichever `Filesystem` you asked for, since most SSDs
  and USB drives ship exFAT already, and if one already holds your
  app's volume that data is the reason it was plugged in. Neither FAT
  variant is crash-safe: both carry the same caveats as `/data` — no
  unix permissions, ownership, symlinks or hard links, and not
  power-loss-robust without the fsync sequence above. A FAT32 volume
  also honors the same `GOSD_DATA_FLUSH` setting as `/data` (`gosd build
  --data-flush`/the card's `config/data_flush`); exFAT never took the
  `flush` mount option and is unaffected either way.
- **Not every board's kernel can mount exFAT.** `CONFIG_EXFAT_FS` is
  required, and `COMPATIBILITY.md`'s "exFAT on attached disks" row says
  which boards have it in their published artifacts; the preflight and
  `ErrUnsupportedFS` behavior are identical to ext4's, above.
- **Both FAT-family formatters are GoSD's own.** `go-diskfs` writes our
  FAT32; `internal/diskfmt` writes exFAT directly from the Microsoft
  specification, since `go-diskfs` has no exFAT support — pure Go, no
  `mkfs.exfat`, no root.
- **Label length depends on the filesystem**: ext4's is 16 ASCII
  characters (its `s_volume_name` field), FAT32/exFAT's is 11 (FAT's own
  limit, and equally valid as an exFAT label) — pass a label longer than
  the filesystem you're asking for allows and `FormatAndMountWith`
  rejects it before touching the disk.

A label match against a *different* filesystem than requested — a drive
that arrived pre-formatted some other way, or an app whose `Filesystem`
choice changed across an upgrade — is never silently converted: it's
treated like any other foreign content, refused unless
`Destructive: true`, exactly like a label that doesn't match at all.

## Logging

There is no syslog, no log file, and no remote log shipping. `/app`'s
stdout and stderr are connected straight to the serial console —
whatever your app prints is what shows up when someone has a serial
cable attached (or a serial-over-USB/console viewer for their board).
Log to stdout/stderr as you normally would; there's nowhere else for it
to go, and nothing else reads it live.

`gosd-init` does, however, quietly retain the last 64KiB of it: if your app
exits unexpectedly, that tail becomes the technical detail in a
[crash report](crash-reports.md) written to the boot partition, so an
unattended device's owner has something to go on even with no serial cable
ever attached. This changes nothing about what reaches the console itself.

## Serial console baud rate (`--console-baud`)

Every board's kernel talks to its debug UART at a fixed default rate
baked into the boot config `gosd build` renders: 115200 on the Pi
boards, 1500000 (1.5Mbaud) on the Rockchip boards (Radxa Zero 3E,
NanoPi Zero2, Radxa ROCK 4SE). Some common USB-serial adapters —
notably CP210x and PL2303 families — can't reliably read 1.5Mbaud;
garbled or missing console output on an otherwise-working board is a
strong signal you've hit this rather than a real boot failure. See
COMPATIBILITY.md's Radxa Zero 3E serial footnote for the specific case
this was found on.

`gosd build --console-baud <rate>` bakes a different rate into the same
config, for every board that renders one:

```sh
gosd build . --board radxa-zero-3e --console-baud 115200
```

- **Only the rate changes.** The UART device itself (`ttyS2`, `serial0`,
  etc.) is fixed per board and unaffected by this flag.
- **Any positive integer is accepted.** A rate outside the common set
  (9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600, 1500000,
  3000000) prints a warning rather than failing outright — a mismatched
  or unusual rate is far more often a typo than an intentional choice,
  but the flag's entire purpose is accommodating hardware GoSD can't
  enumerate in advance, so it doesn't hard-block anything positive.
- **`qemu-virt` can't honor it.** That profile's console is a fixed
  `qemu-system-aarch64 -append "console=ttyAMA0"` argument with no baud
  rate at all (a virtual console has no real adapter to mismatch rates
  with) — see `internal/qemurun`. `--console-baud` together with
  `--board=qemu-virt` fails fast with an actionable error rather than
  silently doing nothing.
- **U-Boot's own output is unaffected.** On the Rockchip boards, U-Boot
  itself prints at its compiled-in 1500000 regardless of
  `--console-baud` — the flag only changes what the *kernel and
  onward* (gosd-init, your app's stdout/stderr) render into
  extlinux.conf/cmdline.txt. If you need U-Boot's own boot log readable
  too, use an adapter that supports 1.5Mbaud (or compile a custom
  U-Boot with a different `CONFIG_BAUDRATE`, out of scope for this
  flag).
- **No reflash needed to try a different rate on an already-flashed
  card.** `extlinux/extlinux.conf` (Rockchip boards) or `cmdline.txt`
  (Pi boards) on the boot partition is a plain text file —
  hand-editing the `console=` argument there has the same effect as
  rebuilding with `--console-baud`.

## Build constraints

- `gosd build` always cross-compiles with `CGO_ENABLED=0` and
  `GOOS=linux`; `GOARCH` (and `GOARM`, where it applies) is chosen per
  board — see `internal/build/build.go` and `internal/boards.Arch` for
  the current mapping. cgo would introduce a dependency on the host's C
  toolchain/libc that the image can't provide, so pure Go dependencies
  only.
- The path you pass to `gosd build` must be a `package main` with a
  `func main` — `gosd build` checks this up front and fails with an
  actionable error otherwise.
- `gosd-init` itself has no shell, no interactive surface, and no
  remote debug access, on purpose — the only things running alongside
  your app are the supervisor, its network/time-sync bring-up, its mDNS
  responder, whichever `--ingress` agent the image was built with
  (`cloudflared` or `tailscale-funnel` — both dial out rather than bind a
  socket on any real host interface, so neither adds a listener), and,
  later, an update listener. If you need to inspect a running device, that
  has to happen through your own app (an HTTP endpoint, for instance, as
  `examples/hello` does) or the serial console.
- Two Go build tags are passed to your app's compile — `gosd`, set for
  every image gosd builds, and the selected board's own
  (`gosd_<board-id>`, e.g. `gosd_pi_zero_2w`). gosd-init is never
  tagged. See [how to gate app source with them](board-build-tags.md) —
  `//go:build gosd` for device-only source, `//go:build !gosd` for the
  desktop/CI fallback.

## Bundling a companion binary (`--with-external`)

Not everything your app needs is a pure-Go library. A video decoder, a
vendor CLI, or any other prebuilt executable can ride along in the same
image via `gosd build --with-external <path>[:<dest>]` (repeatable).
Don't have that binary built yet? `gosd build-external` cross-compiles
one from a `gosd-external.toml` recipe inside Docker/Podman, ready to
hand straight to `--with-external` — see
[`docs/externals.md`](externals.md).

```sh
gosd build . --board pi-zero-2w \
  --with-external ./build/mpv \
  --with-external ./build/tool:/usr/local/bin/tool
```

- **Dest defaults to `/bin/<basename of path>`.** An explicit `<dest>`
  must be an absolute path; one that collides with `/init`, `/app`,
  `/etc/gosd/*`, `/lib/firmware/*`, or another `--with-external`'s dest
  is rejected before the build touches the network or the toolchain.
- **The binary must be fully static.** The initramfs ships no `ld.so`
  and no library layout, so a dynamically linked binary (one with a
  `PT_INTERP` program header) is rejected at build time with an
  actionable error — build it with `CGO_ENABLED=0` (Go) or full static
  linking (C/C++).
- **It must match the board's architecture.** `gosd build` checks each
  external's ELF class/machine against every selected board's target
  architecture before assembling anything — a mismatched binary fails
  immediately, naming the board, instead of shipping something that
  can't `exec`.
- **Your app owns it at runtime.** gosd-init supervises only `/app`, plus a
  small, fixed set of gosd-*shipped* system services (currently
  `cloudflared` and `tailscale-funnel`, each started only when an image is
  built with the matching `--ingress` value — a narrow carve-out to
  gosd-init's original single-child design, see epic gosd-oyhi). It never
  supervises a *user*-provided companion binary:
  launch, monitor, and restart yours yourself via `os/exec`; if the pair
  wedges, exit `/app` and let gosd-init's own backoff supervisor restart
  the unit.

## GPIO, I2C, SPI

GoSD doesn't ship its own hardware I/O library — use the same pure-Go
libraries you'd use on any Linux board:

- [`go-gpiocdev`](https://github.com/warthog618/go-gpiocdev) for GPIO
  via the modern `/dev/gpiochipN` character-device API.
- [`periph.io`](https://periph.io/) for a broader device driver
  ecosystem (I2C, SPI, and specific sensor/peripheral drivers).

Both are plain Go and work under `CGO_ENABLED=0`, so they cross-compile
the same way your app does. GPIO, I2C, and SPI all have worked examples,
covered below.

### GPIO is available via /dev/gpiochipN

`CONFIG_GPIO_CDEV` is already enabled on every board's kernel, so
`/dev/gpiochipN` character devices for the header/FPC pins exist at boot
on every board with no build flag or device-tree change needed — unlike
I2C and SPI, GPIO needed no per-board enablement work at all. What
differs per board is *numbering*: which chip backs which pins, and
which line offset within that chip a given pin is.

- **Raspberry Pi Zero 2 W / Zero W / 3B** (BCM2837/BCM2835): the whole
  SoC is one chip, `gpiochip0` (54 lines). Its device tree maps lines to
  BCM GPIO numbers 1:1 (`gpio-ranges = <&gpio 0 0 54>`, an identity
  mapping), so `gpiochip0`'s line offset is always the same number as
  the "GPIOn" silkscreened on most Pi pinout diagrams. Physical header
  pin 3 (the I2C bus's SDA line — see the table below) is BCM GPIO2,
  i.e. `gpiochip0` line 2; pin 5 (SCL, GPIO3) is `gpiochip0` line 3.
- **Radxa Zero 3E / NanoPi Zero2** (Rockchip RK3566 / RK3528): the GPIO
  controller is split into up to 5 independently-numbered banks
  (`gpio0`..`gpio4`), each its own `/dev/gpiochipN` in bank order (bank
  0 is `gpiochip0`, bank 1 is `gpiochip1`, and so on — true on both
  boards because nothing else on either SoC registers a GPIO chardev
  ahead of them). Rockchip's own signal names spell out the exact line
  within that chip: `GPIO<bank>_<group><pin>`, where group `A`/`B`/`C`/
  `D` are 0/1/2/3, giving a line offset of `group*8 + pin` *within that
  bank's chip* (not a global line number). The I2C bus's `GPIO1_A0`/
  `GPIO1_A1` signals (Radxa, header pins 3/5) are therefore `gpiochip1`
  lines 0 and 1; the NanoPi's `GPIO1_B2`/`GPIO1_B3` (FPC pins 12/13) are
  `gpiochip1` lines 10 and 11.
- **Radxa ROCK 4SE** (Rockchip RK3399): the same up-to-5-bank convention
  as above, and the same `GPIO<bank>_<group><pin>`/`group*8 + pin`
  naming. Hardware-verified during bring-up (bean `gosd-sz6p`,
  2026-07-23): all five banks enumerate as `gpiochip0`-`gpiochip4`, 32
  lines each. The 40-pin header's `GPIO2_A0` signal (physical pin 27 —
  also the `i2c2` bus's SDA2 line, see the I2C table below) is
  `gpiochip2` line 0.

| Board | Connector | GPIO controller | Worked example: the I2C pins above, as (chip, line) |
|---|---|---|---|
| Raspberry Pi Zero 2 W | 40-pin header | One chip, `gpiochip0` (54 lines) | Pin 3 (GPIO2) → `gpiochip0` line 2; pin 5 (GPIO3) → `gpiochip0` line 3 |
| Raspberry Pi Zero W | 40-pin header | One chip, `gpiochip0` (54 lines) | Same as above |
| Raspberry Pi 3B (and 3B+) | 40-pin header | One chip, `gpiochip0` (54 lines) | Same as above |
| Radxa Zero 3E | 40-pin header | 5 banks, `gpiochip0`-`gpiochip4` | Pin 3 (GPIO1_A0) → `gpiochip1` line 0; pin 5 (GPIO1_A1) → `gpiochip1` line 1 |
| NanoPi Zero2 | 30-pin FPC | 5 banks, `gpiochip0`-`gpiochip4` | FPC pin 12 (GPIO1_B2) → `gpiochip1` line 10; FPC pin 13 (GPIO1_B3) → `gpiochip1` line 11 |
| Radxa ROCK 4SE | 40-pin header | 5 banks, `gpiochip0`-`gpiochip4` (32 lines each) | Pin 27 (GPIO2_A0) → `gpiochip2` line 0 |

**Caution: a BCM GPIO number, a physical pin number, and a gpiochip line
offset are three different numbering schemes that happen to coincide on
the Pi boards and don't anywhere else.** The Pi's `gpiochip0` line ==
BCM GPIO number is a property of *that specific device tree's* identity
`gpio-ranges`, not a rule the kernel enforces generally — a board that
recorded its `gpio-ranges` differently (or any non-Pi board) would
break the coincidence. On the Rockchip boards, the line offset is
always local to its bank's chip (`group*8 + pin`), never a whole-SoC
number, and the *physical* pin position on the header/FPC is a third,
independent numbering fixed only by the board's own wiring — always
check a real pinout diagram or schematic for your board rather than
assuming a pattern carries over from another one.

`examples/gpioinfo` is the worked example: by default it opens every
`/dev/gpiochipN` present and prints a `gpioinfo`(1)-style dump — chip
name/label/line count, then each line's offset, name, direction, and
consumer — entirely read-only, so it's safe to run against unknown
wiring. Setting both `GOSD_GPIO_CHIP` (e.g. `gpiochip1`) and
`GOSD_GPIO_LINE` (e.g. `0`) opts into a second, destructive step: that
one line is requested as an output and toggled a few times, logging
each transition — useful for confirming a chip/line pair against a
multimeter or LED before wiring up real application code. Neither env
var alone does anything; the example never drives a pin unless told
exactly which one. Under `qemu-virt`, the `-M virt` machine has no GPIO
controller, so the enumeration step correctly reports "no GPIO
character devices found" and exits 0 rather than erroring. For real
applications, reach for `go-gpiocdev` directly (as the example does) or
`periph.io`'s higher-level line/pin abstractions.

### I2C is on by default

Every board image `gosd build` produces has one I2C bus enabled and
ready as a `/dev/i2c-N` character device by the time your app starts —
no build flag needed, and there's no opt-out flag today (a config-tree
setting to disable it may come later if a real use case needs the pins
back for plain GPIO). The kernel driver has always been built in on
every board (`CONFIG_I2C_BCM2835`/`CONFIG_I2C_RK3X` plus
`CONFIG_I2C_CHARDEV`); what this adds is the device-tree/`config.txt`
enablement that was previously missing.

| Board | Device | Physical pins | Notes |
|---|---|---|---|
| Raspberry Pi Zero 2 W | `/dev/i2c-1` | Header pins 3 (SDA) / 5 (SCL) | Same pins as `GPIO2`/`GPIO3` on any Pi — the standard Pi I2C position. Using those pins as plain GPIO is unavailable while I2C is enabled. |
| Raspberry Pi Zero W | `/dev/i2c-1` | Header pins 3 (SDA) / 5 (SCL) | Same pins, but by a different mechanism: `dtparam=i2c_arm=on` is a no-op on this board (see the SPI row below) — I2C works because upstream `bcm2835-rpi.dtsi` enables it unconditionally regardless of that line (bean `gosd-dkqb`). |
| Raspberry Pi 3B (and 3B+) | `/dev/i2c-1` | Header pins 3 (SDA) / 5 (SCL) | Same as above. |
| Radxa Zero 3E | `/dev/i2c-3` | 40-pin header pins 3 (SDA) / 5 (SCL) | Same physical header position as the Pi's I2C pins, confirmed against Radxa's own schematic and pinout docs. |
| NanoPi Zero2 | `/dev/i2c-5` | 30-pin FPC pins 12 (SCL) / 13 (SDA) | Confirmed against FriendlyElec's schematic. **Needs an external ~2.2kΩ pull-up on both lines** — unlike the other boards' I2C pins, this bus has no onboard pull-up resistors (FriendlyElec's own schematic note); most breakout boards include their own, but bare sensor modules may not. |
| Radxa ROCK 4SE | `/dev/i2c-7` | 40-pin header pins 3 (SDA7) / 5 (SCL7) | Same physical header position as the Pi's I2C pins. **Hardware-verified** (device ACK from a Qwiic Button, bean `gosd-sz6p`, 2026-07-23). Uniquely among GoSD's boards, two more header I2C buses are enabled and equally hardware-verified: `/dev/i2c-2` on pins 27 (SDA2) / 28 (SCL2), and `/dev/i2c-6` on pins 29 (SCL6) / 31 (SDA6) — note the SCL/SDA pin order flips between buses. Adapter numbers are alias-pinned to controller names and stable (buses 0/1/3/4 exist as internal-only buses; 5 and 8 are disabled controllers). |

On the Pi Zero 2W and 3B, enabling I2C means `config.txt` carries
`dtparam=i2c_arm=on` (Raspberry Pi's own documented mechanism). These two
boards' DTBs are downstream-style, which normally carries the
`__overrides__` node that mechanism patches — expected to work, though not
independently re-verified by parsing the released DTB the way pi-zero-w's
mainline-style DTB was (bean `gosd-dkqb`'s per-board scope note). The Pi
Zero W carries the same line, but it does nothing there — see the table
note above; I2C works anyway, by upstream default rather than by this
mechanism. On the three Rockchip boards, enabling I2C
means the shipped kernel's device tree
enables the relevant `i2cN` controller node(s) — see
`build/boards/radxa-zero-3e/kernel/patches/`,
`build/boards/nanopi-zero2/kernel/patches/`, and
`build/boards/rock-4se/kernel/patches/` if you're curious about the
mechanism, or need to add a similar peripheral enablement yourself.

`examples/i2cscan` is a worked example: it opens every `/dev/i2c-*`
present, scans each bus for a responding device, and additionally
checks for a BME280/BMP280-family sensor's chip-ID response — a common,
cheap way to sanity-check your wiring before writing real sensor code.
For anything past that sanity check, reach for `periph.io` rather than
hand-rolling ioctls the way the example does.

### SPI is on by default

Every board image `gosd build` produces has a `/dev/spidev*` character
device ready by the time your app starts — no build flag needed, and
(as with I2C) there's no opt-out flag today. The kernel driver has
always been built in on every board (`CONFIG_SPI_BCM2835`/
`CONFIG_SPI_ROCKCHIP` plus `CONFIG_SPI_SPIDEV`); what this adds is the
device-tree/`config.txt` enablement that was previously missing.

| Board | Device(s) | Physical pins | Notes |
|---|---|---|---|
| Raspberry Pi Zero 2 W | `/dev/spidev0.0`, `/dev/spidev0.1` | Header pins 19 (MOSI) / 21 (MISO) / 23 (SCLK) / 24 (CE0) / 26 (CE1) | The standard Pi SPI0 position, both chip selects. |
| Raspberry Pi Zero W | `/dev/spidev0.0`, `/dev/spidev0.1` | Same as above | **Different mechanism, and pending an artifact release.** `dtparam=spi=on` is a no-op on this board: it's the one GoSD board built from the mainline-style DTS chain, which has no `__overrides__` node for the Pi firmware to patch (bean `gosd-dkqb`). A kernel-build DTS patch (`build/boards/pi-zero-w/kernel/patches/0003-enable-header-spi.patch`) enables `&spi` directly instead, but that only reaches a real (non-`--artifacts-dir`) build once a new `artifacts/vX.Y.Z` release ships it and `internal/artifacts.Version` is bumped to it — until then, this row is aspirational for pi-zero-w. |
| Raspberry Pi 3B (and 3B+) | `/dev/spidev0.0`, `/dev/spidev0.1` | Same as above | Same as above. |
| Radxa Zero 3E | `/dev/spidev3.0` | 40-pin header pins 19 (MOSI) / 21 (MISO) / 23 (SCLK) / 24 (CS0) | Same physical header position as the Pi's SPI0 pins, confirmed against Radxa's own schematic and pinout docs — but only one chip select: physical pin 26, where a Pi's CE1 would be, is not connected on this board's header, so there is no `/dev/spidev3.1`. |
| NanoPi Zero2 | `/dev/spidev1.0`, `/dev/spidev1.1` | 30-pin FPC pins 16 (CLK) / 17 (MOSI) / 18 (MISO) / 19 (CS0) / 20 (CS1) | Confirmed against FriendlyElec's schematic; both chip selects are routed to the FPC connector. |
| Radxa ROCK 4SE | `/dev/spidev1.0` | 40-pin header pins 19 (MOSI) / 21 (MISO) / 23 (SCLK) / 24 (CS0) | Same physical header position as the Pi's SPI0 pins, per Radxa's own pinout docs. **Schematic-derived, not hardware-verified** — SPI wasn't exercised during the board's hardware bring-up (bean `gosd-sz6p`). Only one chip select is wired up (CS0); the DTS patch adds no second `spidev` child node, so there is no `/dev/spidev1.1`. |

On the Pi Zero 2W and 3B, enabling SPI means `config.txt` carries
`dtparam=spi=on` (Raspberry Pi's own documented mechanism, giving both
`spidev0.0` and `spidev0.1`). As with I2C above, these two boards'
downstream-style DTBs normally carry the `__overrides__` node that
mechanism patches, but that's expected rather than independently
re-verified (bean `gosd-dkqb`'s per-board scope note). The Pi Zero W needs
a kernel-build DTS patch instead — see the table note above — the same
mechanism the three Rockchip boards use, where the shipped kernel's device
tree enables the relevant `spiN` controller
node and adds a `spidev` child node for each header-routed chip select
— see `build/boards/pi-zero-w/kernel/patches/`,
`build/boards/radxa-zero-3e/kernel/patches/`,
`build/boards/nanopi-zero2/kernel/patches/`, and
`build/boards/rock-4se/kernel/patches/` if you're curious about the
mechanism. Note the child node's `compatible` value: the kernel's
spidev driver (`drivers/spi/spidev.c`) refuses to bind to a bare
`compatible = "spidev"` node (it logs "spidev listed directly in DT is
not supported" and fails to probe) — GoSD's patches use
`"rohm,dh2228fv"`, spidev's own documented generic placeholder
compatible (`Documentation/spi/spidev.rst`), the same one Raspberry
Pi's downstream spidev overlays use.

`examples/spiloopback` is a worked example: it opens every
`/dev/spidev*` present and performs a full-duplex transfer of a fixed
test pattern, reporting whether the bytes read back match the bytes
sent. This is only a meaningful test with **MOSI physically jumpered to
MISO** on the bus under test — with that jumper in place, a correct
loopback confirms the bus works end-to-end before you wire up a real
device; without it, a mismatch is the expected (not erroneous) result.
For anything past that self-test, reach for `periph.io` rather than
hand-rolling ioctls the way the example does.

## Audio — the `sound` package, and a kernel that has sound in it

Every published GoSD kernel is built with `# CONFIG_SOUND is not set`,
so a stock image has no `/dev/snd` and no app can make a noise. Audio is
in the same position as display: an opt-in `gosd build-kernel` recipe
rather than a base-image feature.

The `sound` package plays interleaved S16_LE frames out of whatever
playback device the board has, talking the kernel's ALSA PCM interface
directly (a GoSD image has no `libasound.so.2` to link or `dlopen`, and
no `/usr/share/alsa` config tree):

```go
dev, err := sound.Open() // or OpenWith(sound.Options{Prefer: sound.Analog})
if err != nil {
	// Wraps sound.ErrNoDevice, and the message names the fix.
	log.Printf("no audio: %v", err)
	return
}
defer func() { _ = dev.Close() }()
err = dev.Play(frames) // len(frames) % dev.Format().FrameBytes() == 0
```

**[docs/sound.md](sound.md) is the full guide**: which recipe each
board needs, what each output physically is, what the kernel grows by,
and the gotchas — HDMI audio only exists if the display was connected
*before* power-up, the Pi Zeros have no analog jack at all, enabling
`CONFIG_SND` drags in every audio driver the defconfig ships as a
module (including a USB MIDI gadget that can claim the only UDC and
break `--usb-gadget`), and the NanoPi Zero2 has no audio path in the
pinned kernel whatsoever.

`examples/chime` is the worked example — a boot chime and a periodic
test tone, plus the kernel recipes for the Pi boards, the ROCK 4SE's
headphone jack, and HDMI audio on the two Rockchip boards that have it.

## USB gadget mode

Your app can present the board as a USB peripheral instead of (or
alongside) its normal role, using the pure-Go `gadget` package — no
cgo, no exec, just configfs file writes. Today that means CDC-ACM
serial and USB mass storage; a USB Ethernet gadget (device-as-network-
interface, no WiFi/cable needed at all) is planned for later.

- Build with `gosd build --usb-gadget` so the board's USB controller
  boots in peripheral mode. On the Pi Zero 2W this repurposes its only
  USB port from host to peripheral mode (the *inner* micro-USB is the
  data port, not the one marked PWR); the Radxa Zero 3E needs no
  flag-driven change at all — its USB-C OTG/power port negotiates role
  automatically.
- `gosd build --usb-gadget` fails fast, naming the offending board, for
  any selected board with no USB peripheral controller at its pinned
  artifacts (e.g. the NanoPi Zero2 today — see COMPATIBILITY.md's USB
  gadget row) rather than producing an image whose app can never find a
  UDC.
- Activation is your app's job, not `gosd-init`'s: construct a
  `gadget.Gadget`, add a `gadget.ACM{}` function, and call `Apply()` at
  startup (`Close()` to tear it down). Without `--usb-gadget` at build
  time, `Apply()` fails with an actionable error instead of silently
  doing nothing.
- Once applied, the device shows up at `/dev/ttyGS0` on the board and
  as a USB-serial device on the host (`/dev/ttyACM0` on Linux,
  `/dev/cu.usbmodem*` on macOS).
- See `examples/usbserial` for a complete worked example: it applies
  the gadget and echoes back every line it reads over `/dev/ttyGS0`.
- A `gadget.MassStorage` function exposes a block device or disk-image
  file on the board as a removable-drive-style disk on the host (one
  LUN, with read-only and removable flags). While it's applied the
  *host* owns that storage outright — never mount or write the backing
  path from the app at the same time; expose or mount, not both.
  `Apply()` enforces this: it refuses (and unwinds cleanly) if `Path` is
  currently mounted, is a partition of a currently-mounted device, or is
  the parent device of a currently-mounted partition, naming the
  mountpoint to `Unmount` first — so a forgotten `Unmount` before handing
  a still-mounted `disk`/`emmc` `BlockDevice` to `MassStorage` fails
  loudly instead of corrupting the volume. It needs
  `CONFIG_USB_CONFIGFS_MASS_STORAGE=y` in the board kernel; see
  COMPATIBILITY.md's USB gadget footnote for per-board status.
- **A LUN is the whole volume — only share one your app owns
  outright.** There is no way to expose a subdirectory: the host gets
  every file on the backing device, dot-prefixed or not, and unless you
  set `ReadOnly` it may write to all of them. `ReadOnly`'s zero value is
  `false`, so an omitted field means read-write; write it out rather
  than letting it be assumed.
- **The boot partition is refused for you.** As it boots, `gosd-init`
  writes down the devices GoSD keeps for the board's own operation, and
  `MassStorage` refuses any `Path` that would hand one to a host —
  the boot partition itself, or the whole card it sits on, since a LUN
  over the disk contains the partition. The refusal holds whether or not
  anything is mounted at the time, which is what makes it a rule rather
  than a coincidence, and it wraps `gadget.ErrReservedDevice` so an app
  that can carry on without a drive can match it with `errors.Is` and do
  so. It is a floor, not a review of your choice of volume: the data
  partition is yours, so it is deliberately not on that list, and the
  bullet below is still yours to honour.
- **Do not share the data partition.** `gosd-init` keeps this device's
  own settings on it, under `/data/.gosd`, so that re-flashing the card
  doesn't lose them — and those settings include, in plain text, the
  passphrase of the WiFi network the board is on, any ingress token, and
  the Tailscale node's private key. Handing that partition to a USB host
  discloses all of it to whatever computer the cable reaches, with no
  case to open and no card to remove, and read-write access lets that
  computer write into `/data/.gosd` too — where it survives the owner's
  next re-flash. The same goes for serving it: `http.FileServer` has no
  notion of a hidden file, so `http.Dir("/data")` publishes
  `/.gosd/config/values/wifi/passphrase` to anyone who can reach the
  port. Give your app a directory of its own under `/data` and serve
  that.
- See `examples/usbwebsite` for a worked example: it serves a static
  website, and presents the volume holding it as a USB drive when
  plugged into a computer so the site can be edited. The volume is the
  onboard eMMC where one is fitted (`emmc.FormatAndMount` returns the
  device backing the mount, and `emmc.Unmount` releases it so
  `gadget.MassStorage` can take it exclusively), which the app owns
  outright and shares freely. Otherwise it is the SD card's data
  partition (build with `--data-size`), which is how the eMMC-less Pi
  Zeros run it — and which the app serves from a `website` folder of its
  own and never offers over USB unless the operator sets a documented
  app setting to accept the disclosure above. The `disk` package pairs
  with `gadget.MassStorage` the same way, for an app that wants to share
  an attached SSD or USB drive instead.

## Testing your app under qemu (no hardware needed)

You don't need a Pi or a Radxa on your desk to see your app run through
the whole boot sequence above — `--board=qemu-virt` builds the same
kind of image for `qemu-system-aarch64 -M virt` instead of real
hardware. The fastest way to use it is `gosd run`, which builds and
boots in one step:

```
go run ./cmd/gosd run ./examples/hello
```

This is an internal/CI board (see CLAUDE.md's locked decisions) — it's
never built by a plain `gosd build` with no `--board`, and it's not a
target you'd ship to end users — but it runs the real `gosd-init`, the
real boot sequence, and your real app, under an emulator instead of an
SD card. `qemu-system-aarch64` needs installing first:

- macOS: `brew install qemu`
- Debian/Ubuntu: `apt-get install qemu-system-arm`

`gosd run` cross-compiles your app and gosd-init, assembles a
qemu-virt image into a temp directory (reusing the exact same build
pipeline, and artifact cache, as `gosd build`), then boots it with
serial console on stdio, so `gosd-init`'s boot log and your app's
stdout/stderr print live in your terminal exactly as they would over a
real serial cable. Your app's port 80 is reachable at
`http://localhost:8080` once gosd-init starts it and networking comes
up (virtio-net, DHCP from qemu's own user-mode network). Ctrl-C stops
qemu and deletes the temp image; pass `--keep` to keep it instead.
Useful flags:

- `--port` changes the host port your app's HTTP port 80 is forwarded
  to (default 8080).
- `--memory` changes the guest's RAM in MiB (default 512).
- `--qemu-arg` passes an extra argument straight through to
  `qemu-system-aarch64` (repeatable) — an escape hatch for anything the
  above doesn't cover.
- `--artifacts-dir`, `--gosd-init-src` and `--data-size` work exactly as
  they do for `gosd build`, for testing against a locally-built kernel or
  gosd-init checkout, or exercising a data-partition-dependent feature (e.g.
  `--ingress tailscale-funnel`) under qemu.

If you already have a `--board=qemu-virt` image built (e.g. from CI, or
because you want to boot the exact same image repeatedly without
rebuilding), `scripts/qemu-run.sh <path-to-image.img>` boots it
directly, using the same underlying qemu invocation
(`internal/qemurun`) as `gosd run`:

```
go run ./cmd/gosd build ./examples/hello --board=qemu-virt -o dist/
scripts/qemu-run.sh dist/hello-qemu-virt.img
```

Quit either one with Ctrl-A X (inside the qemu console), or Ctrl-C to
stop qemu from the host side.

## When to reach for gokrazy instead

GoSD is heavily inspired by [gokrazy](https://gokrazy.org/) — if you
haven't used it, it's worth knowing about regardless of which one you
pick. The honest comparison:

- **Multiple services on one device, or a fleet you update over the
  air:** gokrazy is built around running several Go programs together
  on one device and updating them remotely; that's its core strength.
  GoSD currently runs exactly **one** app per image, and update tooling
  isn't built yet — it's aimed at a single-purpose appliance you
  reflash, not a managed fleet.
- **A wider range of boards, or x86:** gokrazy supports a broad set of
  targets. GoSD is deliberately narrow: a small set of Raspberry Pi,
  Radxa, and FriendlyElec boards (see `COMPATIBILITY.md`), at least for
  now.
- **A one-file, hand-a-friend-an-SD-card appliance, optimized for the
  smallest/cheapest boards:** this is GoSD's focus — minimal image,
  fast boot, no persistence to worry about, and (once the artifact
  pipeline and flashing guide land) a flow a non-technical person can
  follow with the Raspberry Pi Imager.

If you're not sure, gokrazy is the more mature, more general-purpose
choice today. GoSD is worth trying when you want the smallest possible
single-purpose device image and don't need multiple services or fleet
management.
