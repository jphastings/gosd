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

`gosd-init` sets exactly two environment variables before starting `/app`
(see `cmd/gosd-init/internal/boot/sequence.go`):

- `GOSD_BOARD` — the board ID the image was built for (e.g. `pi-zero-2w`),
  as recorded in `config.json` (and overridable at boot via the `gosd.board=`
  kernel command-line parameter).
- `GOSD_HOSTNAME` — the hostname `gosd-init` just applied via `sethostname(2)`.

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

As of today, `gosd-init`'s network bring-up drives **wired Ethernet only**
(interfaces matching `eth*`, `end*`, `enp*` — see
`cmd/gosd-init/internal/netup/netup.go`). `gosd build --wifi-ssid` /
`--wifi-pass` bake WPA2-PSK/open credentials into the image's
`config.json`, but `gosd-init` does not yet read them or associate to a
WiFi network — that association step is still to be built. Don't rely on
WiFi coming up on `main` today; build and test against wired Ethernet, or
check `cmd/gosd-init/internal/netup` for the current state before depending
on WiFi in your app.

## Clock: starts at 1970 until SNTP lands

Neither supported board has a battery-backed real-time clock. On boot, the
system clock starts at the Unix epoch and only becomes correct once
something sets it.

Time sync over SNTP is **planned, not yet built** (tracked as bean
`gosd-c8oj`). Until it exists:

- Anything that checks certificate validity periods (TLS handshakes,
  `crypto/x509` verification) will fail if attempted before the clock is
  correct, because the clock may still read 1970.
- There is currently no signal your app can wait on for "the clock is now
  correct" — no `/run/gosd/time-synced` marker exists yet. When SNTP lands,
  that (or gating on retry) is expected to be the mechanism; until then, the
  safest approach is to retry TLS-dependent operations on failure rather
  than treating an early failure as permanent, and not to hard-fail your app
  if a certificate check fails once shortly after boot.

## Storage: RAM only, `/boot` read-only

GoSD's boot sequence never leaves the initramfs: there's no `pivot_root` or
`switch_root` to a separate root filesystem. The root filesystem your app
runs on is Linux's initramfs `rootfs` — a RAM-backed, writable filesystem —
so:

- Anything your app writes to disk (outside `/boot`) is writable at runtime,
  but **lives in RAM and is gone on reboot or power loss.** There is no
  persistent storage yet. A persistence mechanism is planned for a later
  release (tracked under the `gosd-g...` line of beans) — don't design your
  app around durable local writes until that lands.
- `/boot` — the `GOSD-BOOT` FAT partition containing the kernel, initramfs,
  and boot configuration — is mounted **read-only**. Don't expect to write
  to it from your app.
- Because everything is RAM-resident, be mindful of memory: both supported
  boards are small, memory-constrained devices, and anything you write to
  disk is really consuming RAM.

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
  debug access, on purpose — the only thing running alongside your app is
  the supervisor and (later) mDNS/update listeners. If you need to inspect
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
