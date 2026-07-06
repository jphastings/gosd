# Runtime contract for GoSD apps

This page is the mental model a Go developer needs to write an app that
behaves well on a GoSD image. It describes what actually exists on `main`
today; where something is planned but not yet built, that's called out
explicitly rather than described as if it worked.

Your app is compiled with `CGO_ENABLED=0` for `linux/arm64` (see
`internal/build`) and copied into the image as `/app`. There is no other
userspace: no shell, no init system beyond `gosd-init` itself, no package
manager, no SSH. Whatever your Go binary does is the whole system.

## Supervision

`gosd-init` runs as PID 1. After early setup (mounting `/dev`, `/proc`,
`/sys`, `/run`, setting the hostname, mounting the boot partition) it starts
`/app` and supervises it for the rest of the device's life:

- If `/app` exits, `gosd-init` restarts it automatically. Restarts back off
  (a capped exponential delay) so a fast crash loop doesn't spin the CPU or
  flood the serial console; running stably for a while resets the backoff.
  See `cmd/gosd-init/internal/boot/supervisor.go` and `backoff.go` for the
  exact policy if you need it.
- There's no supervisor-level restart limit — your app is expected to run
  forever, or to be restarted forever if it can't.
- `/app`'s stdout and stderr are connected directly to the serial console
  (see Logging below); `gosd-init`'s own log lines go to the same console,
  each prefixed `[gosd] ` so the two are easy to tell apart.
- If something in early boot fails fatally (e.g. the boot partition never
  mounts), `gosd-init` logs the error, syncs, and reboots the device. Your
  app is never left running with a boot sequence half-completed.

## Environment variables

`gosd-init` sets these environment variables before starting `/app`
(see `cmd/gosd-init/internal/boot/sequence.go`):

- `GOSD_BOARD` — the board ID the image was built for (e.g. `pi-zero-2w`),
  as recorded in `config.json` (and overridable at boot via the `gosd.board=`
  kernel command-line parameter).
- `GOSD_HOSTNAME` — the hostname `gosd-init` just applied via `sethostname(2)`.
- `GOSD_DATA` — the mount point of the writable `GOSD-DATA` partition
  (`/data`), **only set when that partition exists and mounted**. Images
  built with `--data-size=0`, or with a gosd from before the data partition
  existed, boot normally with `GOSD_DATA` unset — check for it rather than
  assuming it. See "Persistent storage" below.

There is deliberately no `GOSD_IP` or similar. Networking comes up
asynchronously after `/app` has already started (see below), so no address
is known at the time `/app` launches. If your app needs its own address,
discover it at runtime with `net.InterfaceAddrs()` / `net.Interfaces()`
rather than expecting it to be handed to you.

## Networking comes up after your app does

`gosd-init` never blocks `/app`'s startup on networking. Network bring-up
(link up, DHCP, DNS, and reacting to a cable being pulled/replugged) runs in
its own goroutine, started just before `/app` is launched, not before.

Practical implications for your app:

- **Never assume connectivity at startup.** Retry any network operation
  (dialing out, listening for inbound connections that depend on routing,
  etc.) rather than treating a failure at process start as fatal.
- **`/run/gosd/network-up`** is an empty marker file `gosd-init` creates once
  an interface has a usable address, and removes if that link later goes
  down (see `cmd/gosd-init/internal/netup/resolvconf.go`). Polling for its
  existence is a reasonable way to gate work that specifically needs an
  address, but plain retry-on-failure works too and doesn't need you to poll
  the filesystem.
- **DNS** is written to `/etc/resolv.conf` from the DHCP lease once one is
  obtained; it's simply absent before then.

`gosd-init` brings up wired Ethernet (interfaces matching `eth*`, `end*`,
`enp*` — see `cmd/gosd-init/internal/netup/netup.go`) and, if the board has
WiFi hardware, associates to a single WPA2-PSK or open network (see
`cmd/gosd-init/internal/wifiup`) using the same DHCP/DNS bring-up either
way. WPA3/EAP networks are out of scope through v0.x; `gosd-init` logs
clearly and skips WiFi bring-up rather than attempting to join one.

The network to join comes from whichever of these sources is present, in
this precedence order (highest wins):

1. **`gosd.toml`**'s `[wifi]` table — the hand-editable fallback on the
   `GOSD-BOOT` partition (see "Provisioning" below).
2. **Cloud-init provisioning** written by Raspberry Pi Imager — also
   below.
3. **`config.json`**, baked at build time by `gosd build --wifi-ssid` /
   `--wifi-pass`.

## Provisioning: hostname and WiFi from Raspberry Pi Imager

Beyond `config.json` baked in at build time and `gosd.toml` hand-edited on
the card, `gosd-init` also reads whatever Raspberry Pi Imager's
customization wizard wrote to the `GOSD-BOOT` partition — cloud-init's
`user-data` (hostname) and `network-config` (WiFi access points) — see
`internal/provision` and `docs/provisioning-formats.md` for the full field
mapping and precedence rationale. This is the flagship end-user flashing
path: publish a custom-repository catalog entry for your image
(`init_format: "cloudinit"`) and Imager's full WiFi/hostname wizard becomes
available for anyone flashing it.

Practical notes:

- `gosd-init` only ever consumes hostname and WiFi from these files —
  everything else the wizard can configure (users, SSH keys, locale,
  passwordless sudo, ...) is RPi-OS-specific and silently ignored, since
  `gosd-init` has no shell or user accounts to apply them to.
- `firstrun.sh` (Imager's older, non-cloud-init mechanism) is **never**
  parsed or executed — if one is found on the boot partition, `gosd-init`
  logs a line pointing you at `gosd.toml` instead.
- A malformed or partially-written provisioning file is logged and
  skipped, falling back to the next-lower-precedence source; it never
  blocks boot.

## Clock: starts at 1970 until SNTP syncs

Neither supported board has a battery-backed real-time clock. On boot, the
system clock starts at the Unix epoch and only becomes correct once SNTP
sync completes.

`gosd-init` syncs the clock itself (`cmd/gosd-init/internal/timesync`) —
your app doesn't need to do anything to make this happen:

- Once `/run/gosd/network-up` appears, `gosd-init` queries NTP (retrying
  with backoff until the first success), sets the clock via
  `settimeofday`, and re-syncs hourly afterwards for the life of the
  device.
- The server list comes from `config.json`'s optional `ntpServers` field
  (baked in by `gosd build`); when it's absent — including every image
  built before this field existed — it defaults to `pool.ntp.org`.
- **`/run/gosd/time-synced`** is an empty marker file `gosd-init` creates on
  the first successful sync. Gate anything that checks certificate validity
  periods (TLS handshakes, `crypto/x509` verification) on this file existing
  — attempting those before the clock is correct fails, because the clock
  may still read 1970. Polling for the marker or simply retrying
  TLS-dependent operations on failure both work; either way, don't treat an
  early failure as permanent, since the clock does become correct within
  moments of the network coming up.

## Storage: RAM rootfs, `/boot` read-only, `/data` persistent

GoSD's boot sequence never leaves the initramfs: there's no `pivot_root` or
`switch_root` to a separate root filesystem. The root filesystem your app
runs on is Linux's initramfs `rootfs` — a RAM-backed, writable filesystem —
so:

- Anything your app writes outside `/data` and `/boot` is writable at
  runtime, but **lives in RAM and is gone on reboot or power loss.** For
  durable writes, use the `GOSD_DATA` partition (below).
- `/boot` — the `GOSD-BOOT` FAT partition containing the kernel, initramfs,
  and boot configuration — is mounted **read-only**. Don't expect to write
  to it from your app.
- Because the rootfs is RAM-resident, be mindful of memory: both supported
  boards are small, memory-constrained devices, and anything you write to
  the rootfs is really consuming RAM.

## Persistent storage: `/data`

Images are built with a second FAT32 partition, labelled `GOSD-DATA`, sized
by `gosd build --data-size` (1GiB unless you say otherwise; `--data-size=0`
omits it). `gosd-init` mounts it read-write at `/data` and sets
`GOSD_DATA=/data` in your app's environment. Data written there survives
reboots and power cycles.

Rules of engagement:

- **Gate on `GOSD_DATA` being set.** If the image was built with
  `--data-size=0` (or by an older gosd), the partition doesn't exist,
  `GOSD_DATA` is unset, and `/data` isn't mounted — boot still proceeds
  normally. A well-behaved app treats "no `GOSD_DATA`" as "no persistence
  available" rather than failing.
- **It's FAT32, with FAT32's limits.** No unix permissions, no ownership,
  no symlinks or hard links, 4GiB max file size, coarse (2s) mtime
  granularity. Don't design around any of those existing.
- **It is not power-loss-robust.** FAT has no journal. The partition is
  mounted with the `flush` option so data reaches the card promptly, but a
  power cut mid-write can still corrupt the file being written (and, less
  commonly, the filesystem). Write durable state the boring, robust way:
  write to a temporary name, `fsync` the file, then `rename` it over the
  real name — readers then always see either the old version or the new
  one. Never rewrite your only copy of something in place.
- **`/data/.gosd-data`** is an empty marker file `gosd-init` creates the
  first time the partition mounts; leave it alone, and don't be surprised
  by it when listing `/data`.
- **Reflashing wipes `/data`.** In v0.3, flashing a new image version
  recreates the data partition from scratch — everything your app stored is
  gone. This is deliberate for now. The planned app-slot update mechanism
  (`docs/design/ab-updates.md`) changes only files inside `GOSD-BOOT` and
  never touches the partition table, so once it lands, over-the-network app
  updates will leave `GOSD-DATA` intact — it's a full reflash, and only a
  full reflash, that wipes it.

For a worked example, `examples/hello` persists a boot counter to
`GOSD_DATA` using exactly the write-rename-fsync pattern above.

## Logging

There is no syslog, no log file, and no remote log shipping. `/app`'s
stdout and stderr are connected straight to the serial console — whatever
your app prints is what shows up when someone has a serial cable attached
(or a serial-over-USB/console viewer for their board). Log to stdout/stderr
as you normally would; there's nowhere else for it to go, and nothing else
reads it.

## Build constraints

- `gosd build` always cross-compiles with `CGO_ENABLED=0`,
  `GOOS=linux`, `GOARCH=arm64` (see `internal/build/build.go`) — both
  supported boards are arm64, and cgo would introduce a dependency on the
  host's C toolchain/libc that the image can't provide. Pure Go dependencies
  only.
- The path you pass to `gosd build` must be a `package main` with a `func
  main` — `gosd build` checks this up front and fails with an actionable
  error otherwise.
- `gosd-init` itself has no shell, no interactive surface, and no remote
  debug access, on purpose — the only things running alongside your app are
  the supervisor, its network/time-sync bring-up, and its mDNS responder
  (and, later, an update listener). If you need to inspect
  a running device, that has to happen through your own app (an HTTP
  endpoint, for instance, as `examples/hello` does) or the serial console.

## GPIO, I2C, SPI

GoSD doesn't ship its own hardware I/O library — use the same pure-Go
libraries you'd use on any Linux board:

- [`go-gpiocdev`](https://github.com/warthog618/go-gpiocdev) for GPIO via
  the modern `/dev/gpiochipN` character-device API.
- [`periph.io`](https://periph.io/) for a broader device driver ecosystem
  (I2C, SPI, and specific sensor/peripheral drivers).

Both are plain Go and work under `CGO_ENABLED=0`, so they cross-compile
the same way your app does. Worked, board-tested examples wiring these up
under GoSD are planned for v0.3; until then, treat the libraries' own
examples as your starting point and adjust device paths for your board.

## USB gadget mode

Your app can present the board as a USB peripheral instead of (or alongside)
its normal role, using the pure-Go `gadget` package — no cgo, no exec, just
configfs file writes. Today that means CDC-ACM serial; a USB Ethernet
gadget (device-as-network-interface, no WiFi/cable needed at all) is
planned for later.

- Build with `gosd build --usb-gadget` so the board's USB controller boots
  in peripheral mode. On the Pi Zero 2W this repurposes its only USB port
  from host to peripheral mode (the *inner* micro-USB is the data port, not
  the one marked PWR); the Radxa Zero 3E needs no flag-driven change at all
  — its USB-C OTG/power port negotiates role automatically.
- Activation is your app's job, not `gosd-init`'s: construct a
  `gadget.Gadget`, add a `gadget.ACM{}` function, and call `Apply()` at
  startup (`Close()` to tear it down). Without `--usb-gadget` at build time,
  `Apply()` fails with an actionable error instead of silently doing
  nothing.
- Once applied, the device shows up at `/dev/ttyGS0` on the board and as a
  USB-serial device on the host (`/dev/ttyACM0` on Linux, `/dev/cu.usbmodem*`
  on macOS).
- See `examples/usbserial` for a complete worked example: it applies the
  gadget and echoes back every line it reads over `/dev/ttyGS0`.

## Testing your app under qemu (no hardware needed)

You don't need a Pi or a Radxa on your desk to see your app run through the
whole boot sequence above — `--board=qemu-virt` builds the same kind of
image for `qemu-system-aarch64 -M virt` instead of real hardware, and
`scripts/qemu-run.sh` boots it for you:

```
go run ./cmd/gosd build ./examples/hello --board=qemu-virt -o dist/
scripts/qemu-run.sh dist/hello-qemu-virt.img
```

This is an internal/CI board (see CLAUDE.md's locked decisions) — it's
never built by a plain `gosd build` with no `--board`, and it's not a
target you'd ship to end users — but it runs the real `gosd-init`, the
real boot sequence, and your real app, under an emulator instead of an SD
card. `qemu-system-aarch64` needs installing first:

- macOS: `brew install qemu`
- Debian/Ubuntu: `apt-get install qemu-system-arm`

`scripts/qemu-run.sh` extracts the kernel `Image` and `initramfs.cpio.zst`
straight out of the image's FAT boot partition (no root, no mtools — see
`internal/cmd/imgextract`), then launches qemu with serial console on
stdio, so `gosd-init`'s boot log and your app's stdout/stderr print live in
your terminal exactly as they would over a real serial cable. Your app's
port 80 is reachable at `http://localhost:8080` once gosd-init starts it
and networking comes up (virtio-net, DHCP from qemu's own user-mode
network). Quit with Ctrl-A X, or Ctrl-C to force-kill qemu.

## When to reach for gokrazy instead

GoSD is heavily inspired by [gokrazy](https://gokrazy.org/) — if you haven't
used it, it's worth knowing about regardless of which one you pick. The
honest comparison:

- **Multiple services on one device, or a fleet you update over the air:**
  gokrazy is built around running several Go programs together on one
  device and updating them remotely; that's its core strength. GoSD
  currently runs exactly **one** app per image, and update tooling isn't
  built yet — it's aimed at a single-purpose appliance you reflash, not a
  managed fleet.
- **A wider range of boards, or x86:** gokrazy supports a broad set of
  targets. GoSD is deliberately narrow: two arm64 boards (Pi Zero 2 W,
  Radxa Zero 3E), at least for now.
- **A one-file, hand-a-friend-an-SD-card appliance, optimized for the
  smallest/cheapest boards:** this is GoSD's focus — minimal image, fast
  boot, no persistence to worry about, and (once the artifact pipeline and
  flashing guide land) a flow a non-technical person can follow with the
  Raspberry Pi Imager.

If you're not sure, gokrazy is the more mature, more general-purpose choice
today. GoSD is worth trying when you want the smallest possible
single-purpose device image and don't need multiple services or fleet
management.
