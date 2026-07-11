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

There's deliberately no `GOSD_DATA`: persistent storage always lives at the
fixed path `/data` (see "Persistent storage" below), so there's nothing to
communicate — write there directly. There is likewise no `GOSD_IP` or similar. Networking comes up
asynchronously after `/app` has already started (see below), so no address
is known at the time `/app` launches. If your app needs its own address,
discover it at runtime with `net.InterfaceAddrs()` / `net.Interfaces()`
rather than expecting it to be handed to you.

## App environment variables (`gosd.toml [env]`)

Beyond the `GOSD_*` vars above, your app can also receive whatever plain
key/value settings its deployment needs — read them the normal way, with
`os.Getenv`. There's nothing GoSD-specific about consuming them; the
GoSD-specific part is where the values come from. `examples/hello` reads
an optional `GREETING` var this way (see its `main.go`).

There are two sources, and `gosd-init` merges them **per key** (not as a
whole-map replace) before starting `/app` — see `mergeUserEnv` in
`cmd/gosd-init/internal/boot/sequence.go`:

1. **`gosd.toml`'s `[env]` table** — the hand-editable fallback on the
   `GOSD-BOOT` partition (see "Provisioning" below). This wins per key.
2. **Baked defaults** from `gosd build --env KEY=VALUE` (repeatable),
   recorded in `config.json`. These are also pre-filled into the card's
   `gosd.toml [env]` section at build time, so whoever holds the card can
   see the developer's defaults and override any of them without needing
   to know the rest.

Precedence is evaluated per key: if the card sets `LOG_LEVEL` but not
`API_URL`, and a baked default set both, your app gets the card's
`LOG_LEVEL` alongside the baked `API_URL` — not one source or the other in
its entirety.

Your app's environment is otherwise a clean slate: it gets exactly the
`GOSD_*` vars above plus this merged user env, not a copy of `gosd-init`'s
own environment (`os.Environ()`).

**Reserved names.** Keys in `gosd-init`'s own `GOSD_*` namespace (`GOSD_BOARD`,
`GOSD_HOSTNAME`, and any future `GOSD_*` var) can never be set
this way. `gosd build --env` refuses a `GOSD_*` key outright, with an
actionable error, before it ever reaches an image. A `GOSD_*` key
hand-written into a card's `gosd.toml [env]` is logged and ignored at boot
instead — your app always gets `gosd-init`'s real value for those, never
whatever a card tried to override them with.

**Missing or empty is fine.** No `--env` flags at build time and no `[env]`
table on the card is a normal, unremarkable boot: your app just gets none
of these vars (plus the `GOSD_*` ones above), and nothing errors either way.

**Quote your values.** Write `gosd.toml [env]` entries as quoted TOML
strings, e.g. `PORT = "8080"`. A bare scalar (`PORT = 8080`, `DEBUG = true`)
is coerced to its string form and still applied, but logs a one-line
warning at boot; an array, inline table, or datetime under `[env]` is
dropped entirely (also warned, never silently). Quoting up front avoids
relying on that coercion.

**Security note.** Like the WiFi passphrase stored in the same file,
`gosd.toml [env]` values sit in plaintext on the `GOSD-BOOT` FAT partition —
anyone with physical access to the card, or who mounts the image, can read
them. There's no encryption today; don't put anything there you wouldn't
want exposed to whoever holds the card.

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
  durable writes, use the `/data` partition (below).
- `/boot` — the `GOSD-BOOT` FAT partition containing the kernel, initramfs,
  and boot configuration — is mounted **read-only**. Don't expect to write
  to it from your app.
- Because the rootfs is RAM-resident, be mindful of memory: both supported
  boards are small, memory-constrained devices, and anything you write to
  the rootfs is really consuming RAM.

## Persistent storage: `/data`

Images are built with a second FAT32 partition, labelled `GOSD-DATA`, sized
by `gosd build --data-size` (1GiB unless you say otherwise; `--data-size=0`
omits it). `gosd-init` mounts it read-write at the fixed path `/data`. Data
written there survives reboots and power cycles. There's no environment
variable to consult — `/data` is always the path; just write to it.

Rules of engagement:

- **When there's no partition, `/data` is read-only.** If the image was built
  with `--data-size=0`, or the card's data partition can't be mounted (a bad
  card, say), `gosd-init` mounts an empty **read-only** filesystem at `/data`
  instead — boot still proceeds normally. A write then fails immediately with
  `EROFS` rather than silently landing in RAM and vanishing on the next
  reboot. This is deliberate: it turns "I thought I had persistence" into a
  loud error at the write, not silent data loss. A well-behaved app treats an
  `EROFS` write to `/data` as "no persistence available this boot" rather than
  a fatal error.
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
`/data` using exactly the write-rename-fsync pattern above, and reports
"no-data-partition" when the write comes back `EROFS`.

## Onboard eMMC storage (Rockchip boards)

The two Rockchip boards, the Radxa Zero 3E and the NanoPi Zero2, also have a
soldered-on eMMC in addition to the microSD card they boot from — the Pi
boards have no such thing. The public `emmc` package lets your app format
and mount it:

```go
if err := <-emmc.FormatAndMount("APPDATA", "/storage", false); err != nil {
	log.Printf("no persistent storage: %v", err)
}
```

`FormatAndMount` returns immediately; the formatting/mounting work runs in
the background, and the returned channel receives exactly one value — `nil`
once the mountpoint is ready, or an error — before closing. Block on it only
once your app actually needs the storage.

- **The eMMC is discovered automatically**, distinguishing it from the
  microSD card the board is currently running from — the boot device is
  never a format target, so there's no risk of an app wiping the card it's
  running on.
- **Formatting is idempotent, keyed on the label you pass.** An eMMC already
  carrying a FAT filesystem labelled `label` is only mounted, never
  reformatted — this is how a second run (or every run after the first)
  avoids wiping its own data. A blank eMMC (no filesystem at all) is always
  formatted, even with `destructive` set to `false`.
- **`destructive` guards everything else.** If the eMMC holds *other* data —
  a FAT volume under a different label, or non-FAT content — `false` makes
  `FormatAndMount` refuse and return an error rather than touch it; `true`
  wipes and reformats it. `label` is limited to 11 ASCII characters (FAT's
  own volume-label limit) and is stored upper-cased.
- **It's a whole-device FAT filesystem** — the mount source is the raw
  `/dev/mmcblkN` device, not a partition on it — with the same limits as
  `/data`: no unix permissions, ownership, symlinks, or hard links, and it is
  not power-loss-robust. Write durable state with the same temp-file,
  `fsync`, then `rename` pattern described under "Persistent storage: `/data`"
  above.
- **On a board with no onboard eMMC** (the Pi boards, or a Rockchip board
  whose only eMMC turns out to be the boot device), `FormatAndMount`'s
  channel yields `emmc.ErrNoEMMC` — check for it with `errors.Is` and treat
  it as "no eMMC here" rather than a fatal error, the way `examples/emmcstorage`
  does.

`examples/emmcstorage` is the worked example: it formats and mounts the eMMC at
`/storage`, degrades gracefully (logs and exits cleanly) when `ErrNoEMMC`
comes back, and otherwise writes a small file and reads it back to
demonstrate persistence.

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
- Each selected board's own Go build tag (`gosd_<board-id>`, e.g.
  `gosd_pi_zero_2w`) is passed to your app's compile — gosd-init is never
  tagged. See [`docs/board-build-tags.md`](board-build-tags.md) for how to
  gate board-specific source with it.

## GPIO, I2C, SPI

GoSD doesn't ship its own hardware I/O library — use the same pure-Go
libraries you'd use on any Linux board:

- [`go-gpiocdev`](https://github.com/warthog618/go-gpiocdev) for GPIO via
  the modern `/dev/gpiochipN` character-device API.
- [`periph.io`](https://periph.io/) for a broader device driver ecosystem
  (I2C, SPI, and specific sensor/peripheral drivers).

Both are plain Go and work under `CGO_ENABLED=0`, so they cross-compile
the same way your app does. GPIO, I2C, and SPI all have worked examples,
covered below.

### GPIO is available via /dev/gpiochipN

`CONFIG_GPIO_CDEV` is already enabled on every board's kernel, so
`/dev/gpiochipN` character devices for the header/FPC pins exist at boot on
all four boards with no build flag or device-tree change needed — unlike
I2C and SPI, GPIO needed no per-board enablement work at all. What differs
per board is *numbering*: which chip backs which pins, and which line
offset within that chip a given pin is.

- **Raspberry Pi Zero 2 W / Zero W** (BCM2837/BCM2835): the whole SoC is one
  chip, `gpiochip0` (54 lines). Its device tree maps lines to BCM GPIO
  numbers 1:1 (`gpio-ranges = <&gpio 0 0 54>`, an identity mapping), so
  `gpiochip0`'s line offset is always the same number as the "GPIOn"
  silkscreened on most Pi pinout diagrams. Physical header pin 3 (the I2C
  bus's SDA line — see the table below) is BCM GPIO2, i.e. `gpiochip0` line
  2; pin 5 (SCL, GPIO3) is `gpiochip0` line 3.
- **Radxa Zero 3E / NanoPi Zero2** (Rockchip RK3566 / RK3528): the GPIO
  controller is split into up to 5 independently-numbered banks
  (`gpio0`..`gpio4`), each its own `/dev/gpiochipN` in bank order (bank 0 is
  `gpiochip0`, bank 1 is `gpiochip1`, and so on — true on both boards
  because nothing else on either SoC registers a GPIO chardev ahead of
  them). Rockchip's own signal names spell out the exact line within that
  chip: `GPIO<bank>_<group><pin>`, where group `A`/`B`/`C`/`D` are 0/1/2/3,
  giving a line offset of `group*8 + pin` *within that bank's chip* (not a
  global line number). The I2C bus's `GPIO1_A0`/`GPIO1_A1` signals (Radxa,
  header pins 3/5) are therefore `gpiochip1` lines 0 and 1; the NanoPi's
  `GPIO1_B2`/`GPIO1_B3` (FPC pins 12/13) are `gpiochip1` lines 10 and 11.

| Board | Connector | GPIO controller | Worked example: the I2C pins above, as (chip, line) |
|---|---|---|---|
| Raspberry Pi Zero 2 W | 40-pin header | One chip, `gpiochip0` (54 lines) | Pin 3 (GPIO2) → `gpiochip0` line 2; pin 5 (GPIO3) → `gpiochip0` line 3 |
| Raspberry Pi Zero W | 40-pin header | One chip, `gpiochip0` (54 lines) | Same as above |
| Radxa Zero 3E | 40-pin header | 5 banks, `gpiochip0`-`gpiochip4` | Pin 3 (GPIO1_A0) → `gpiochip1` line 0; pin 5 (GPIO1_A1) → `gpiochip1` line 1 |
| NanoPi Zero2 | 30-pin FPC | 5 banks, `gpiochip0`-`gpiochip4` | FPC pin 12 (GPIO1_B2) → `gpiochip1` line 10; FPC pin 13 (GPIO1_B3) → `gpiochip1` line 11 |

**Caution: a BCM GPIO number, a physical pin number, and a gpiochip line
offset are three different numbering schemes that happen to coincide on the
Pi boards and don't anywhere else.** The Pi's `gpiochip0` line == BCM GPIO
number is a property of *that specific device tree's* identity
`gpio-ranges`, not a rule the kernel enforces generally — a board that
recorded its `gpio-ranges` differently (or any non-Pi board) would break the
coincidence. On the Rockchip boards, the line offset is always local to its
bank's chip (`group*8 + pin`), never a whole-SoC number, and the *physical*
pin position on the header/FPC is a third, independent numbering fixed only
by the board's own wiring — always check a real pinout diagram or schematic
for your board rather than assuming a pattern carries over from another
one.

`examples/gpioinfo` is the worked example: by default it opens every
`/dev/gpiochipN` present and prints a `gpioinfo`(1)-style dump — chip
name/label/line count, then each line's offset, name, direction, and
consumer — entirely read-only, so it's safe to run against unknown wiring.
Setting both `GOSD_GPIO_CHIP` (e.g. `gpiochip1`) and `GOSD_GPIO_LINE` (e.g.
`0`) opts into a second, destructive step: that one line is requested as an
output and toggled a few times, logging each transition — useful for
confirming a chip/line pair against a multimeter or LED before wiring up
real application code. Neither env var alone does anything; the example
never drives a pin unless told exactly which one. Under `qemu-virt`, the
`-M virt` machine has no GPIO controller, so the enumeration step correctly
reports "no GPIO character devices found" and exits 0 rather than erroring.
For real applications, reach for `go-gpiocdev` directly (as the example
does) or `periph.io`'s higher-level line/pin abstractions.

### I2C is on by default

Every board image `gosd build` produces has one I2C bus enabled and ready as
a `/dev/i2c-N` character device by the time your app starts — no build flag
needed, and there's no opt-out flag today (a `gosd.toml` knob to disable it
may come later if a real use case needs the pins back for plain GPIO). The
kernel driver has always been built in on every board
(`CONFIG_I2C_BCM2835`/`CONFIG_I2C_RK3X` plus `CONFIG_I2C_CHARDEV`); what this
adds is the device-tree/`config.txt` enablement that was previously missing.

| Board | Device | Physical pins | Notes |
|---|---|---|---|
| Raspberry Pi Zero 2 W | `/dev/i2c-1` | Header pins 3 (SDA) / 5 (SCL) | Same pins as `GPIO2`/`GPIO3` on any Pi — the standard Pi I2C position. Using those pins as plain GPIO is unavailable while I2C is enabled. |
| Raspberry Pi Zero W | `/dev/i2c-1` | Header pins 3 (SDA) / 5 (SCL) | Same as above. |
| Radxa Zero 3E | `/dev/i2c-3` | 40-pin header pins 3 (SDA) / 5 (SCL) | Same physical header position as the Pi's I2C pins, confirmed against Radxa's own schematic and pinout docs. |
| NanoPi Zero2 | `/dev/i2c-5` | 30-pin FPC pins 12 (SCL) / 13 (SDA) | Confirmed against FriendlyElec's schematic. **Needs an external ~2.2kΩ pull-up on both lines** — unlike the other boards' I2C pins, this bus has no onboard pull-up resistors (FriendlyElec's own schematic note); most breakout boards include their own, but bare sensor modules may not. |

On the Pi boards, enabling I2C means `config.txt` carries
`dtparam=i2c_arm=on` (Raspberry Pi's own documented mechanism); on the two
Rockchip boards, it means the shipped kernel's device tree enables the
relevant `i2cN` controller node — see
`build/boards/radxa-zero-3e/kernel/patches/` and
`build/boards/nanopi-zero2/kernel/patches/` if you're curious about the
mechanism, or need to add a similar peripheral enablement yourself.

`examples/i2cscan` is a worked example: it opens every `/dev/i2c-*` present,
scans each bus for a responding device, and additionally checks for a
BME280/BMP280-family sensor's chip-ID response — a common, cheap way to
sanity-check your wiring before writing real sensor code. For anything past
that sanity check, reach for `periph.io` rather than hand-rolling ioctls the
way the example does.

### SPI is on by default

Every board image `gosd build` produces has a `/dev/spidev*` character
device ready by the time your app starts — no build flag needed, and (as
with I2C) there's no opt-out flag today. The kernel driver has always been
built in on every board (`CONFIG_SPI_BCM2835`/`CONFIG_SPI_ROCKCHIP` plus
`CONFIG_SPI_SPIDEV`); what this adds is the device-tree/`config.txt`
enablement that was previously missing.

| Board | Device(s) | Physical pins | Notes |
|---|---|---|---|
| Raspberry Pi Zero 2 W | `/dev/spidev0.0`, `/dev/spidev0.1` | Header pins 19 (MOSI) / 21 (MISO) / 23 (SCLK) / 24 (CE0) / 26 (CE1) | The standard Pi SPI0 position, both chip selects. |
| Raspberry Pi Zero W | `/dev/spidev0.0`, `/dev/spidev0.1` | Same as above | Same as above. |
| Radxa Zero 3E | `/dev/spidev3.0` | 40-pin header pins 19 (MOSI) / 21 (MISO) / 23 (SCLK) / 24 (CS0) | Same physical header position as the Pi's SPI0 pins, confirmed against Radxa's own schematic and pinout docs — but only one chip select: physical pin 26, where a Pi's CE1 would be, is not connected on this board's header, so there is no `/dev/spidev3.1`. |
| NanoPi Zero2 | `/dev/spidev1.0`, `/dev/spidev1.1` | 30-pin FPC pins 16 (CLK) / 17 (MOSI) / 18 (MISO) / 19 (CS0) / 20 (CS1) | Confirmed against FriendlyElec's schematic; both chip selects are routed to the FPC connector. |

On the Pi boards, enabling SPI means `config.txt` carries `dtparam=spi=on`
(Raspberry Pi's own documented mechanism, giving both `spidev0.0` and
`spidev0.1`); on the two Rockchip boards, it means the shipped kernel's
device tree enables the relevant `spiN` controller node and adds a
`spidev` child node for each header-routed chip select — see
`build/boards/radxa-zero-3e/kernel/patches/` and
`build/boards/nanopi-zero2/kernel/patches/` if you're curious about the
mechanism. Note the child node's `compatible` value: the kernel's spidev
driver (`drivers/spi/spidev.c`) refuses to bind to a bare `compatible =
"spidev"` node (it logs "spidev listed directly in DT is not supported" and
fails to probe) — GoSD's patches use `"rohm,dh2228fv"`, spidev's own
documented generic placeholder compatible (`Documentation/spi/spidev.rst`),
the same one Raspberry Pi's downstream spidev overlays use.

`examples/spiloopback` is a worked example: it opens every `/dev/spidev*`
present and performs a full-duplex transfer of a fixed test pattern,
reporting whether the bytes read back match the bytes sent. This is only a
meaningful test with **MOSI physically jumpered to MISO** on the bus under
test — with that jumper in place, a correct loopback confirms the bus works
end-to-end before you wire up a real device; without it, a mismatch is the
expected (not erroneous) result. For anything past that self-test, reach for
`periph.io` rather than hand-rolling ioctls the way the example does.

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
image for `qemu-system-aarch64 -M virt` instead of real hardware. The
fastest way to use it is `gosd run`, which builds and boots in one step:

```
go run ./cmd/gosd run ./examples/hello
```

This is an internal/CI board (see CLAUDE.md's locked decisions) — it's
never built by a plain `gosd build` with no `--board`, and it's not a
target you'd ship to end users — but it runs the real `gosd-init`, the
real boot sequence, and your real app, under an emulator instead of an SD
card. `qemu-system-aarch64` needs installing first:

- macOS: `brew install qemu`
- Debian/Ubuntu: `apt-get install qemu-system-arm`

`gosd run` cross-compiles your app and gosd-init, assembles a qemu-virt
image into a temp directory (reusing the exact same build pipeline, and
artifact cache, as `gosd build`), then boots it with serial console on
stdio, so `gosd-init`'s boot log and your app's stdout/stderr print live in
your terminal exactly as they would over a real serial cable. Your app's
port 80 is reachable at `http://localhost:8080` once gosd-init starts it
and networking comes up (virtio-net, DHCP from qemu's own user-mode
network). Ctrl-C stops qemu and deletes the temp image; pass `--keep` to
keep it instead. Useful flags:

- `--port` changes the host port your app's HTTP port 80 is forwarded to
  (default 8080).
- `--memory` changes the guest's RAM in MiB (default 512).
- `--qemu-arg` passes an extra argument straight through to
  `qemu-system-aarch64` (repeatable) — an escape hatch for anything the
  above doesn't cover.
- `--artifacts-dir` and `--gosd-init-src` work exactly as they do for
  `gosd build`, for testing against a locally-built kernel or gosd-init
  checkout.

If you already have a `--board=qemu-virt` image built (e.g. from CI, or
because you want to boot the exact same image repeatedly without
rebuilding), `scripts/qemu-run.sh <path-to-image.img>` boots it directly,
using the same underlying qemu invocation (`internal/qemurun`) as
`gosd run`:

```
go run ./cmd/gosd build ./examples/hello --board=qemu-virt -o dist/
scripts/qemu-run.sh dist/hello-qemu-virt.img
```

Quit either one with Ctrl-A X (inside the qemu console), or Ctrl-C to stop
qemu from the host side.

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
