# GoSD

Turn a Go application into a flashable SD-card image for the Raspberry Pi
Zero 2W, Raspberry Pi Zero W, Raspberry Pi 3B (and 3B+), Radxa Zero 3E,
FriendlyElec NanoPi Zero2, Radxa ROCK 4SE, and Radxa Cubie A5E.
[`COMPATIBILITY.md`](COMPATIBILITY.md) is the board × feature matrix: what
works where, and how each cell was verified.

Like [gokrazy](https://gokrazy.org), but the result is something _anyone_ can
burn and use: images plug into Raspberry Pi Imager's WiFi/hostname wizard, so
the people you send them to never need a terminal.

## Features

- One CLI, runnable locally or in CI — building an image needs no Docker, no
  root, and no Linux
- Boots to your app in about 10 seconds, and to a wired `hostname.local` in
  about the same — over WiFi expect ~25s, because association, DHCP and mDNS
  all happen after your app is already up
- Runs any normal Linux-capable Go program — no SDK, no special imports
- Networking via Ethernet (DHCP) or WiFi (credentials written as the SD card
  is flashed)
- Optional USB gadget mode — the board presents as a USB serial or
  mass-storage _device_: see
  [`docs/runtime.md`](docs/runtime.md#usb-gadget-mode) and
  `examples/usbserial`

## Quickstart

See [the project's GitHub releases](https://github.com/jphastings/gosd/releases)
for what's shipped; per-board verification status lives in
[`COMPATIBILITY.md`](COMPATIBILITY.md).

1. Install the CLI:

   ```sh
   go install github.com/jphastings/gosd/cmd/gosd@latest
   ```

   Or, with nix — handy in CI, since the flake bundles the Go toolchain and
   a vendored copy of gosd's own sources, so `gosd build` works offline
   apart from your app's dependencies and board artifacts:

   ```sh
   nix run github:jphastings/gosd -- build ./cmd/myapp
   ```

2. Write a `main.go`. GoSD runs any normal Go program — no special imports
   or SDK required:

   ```go
   package main

   import (
       "fmt"
       "net/http"
   )

   func main() {
       http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
           fmt.Fprintln(w, "hello from gosd")
       })
       http.ListenAndServe(":80", nil)
   }
   ```

   See `examples/hello` for a slightly fuller worked example (it also
   reports hostname and uptime, and falls back to `:8080` if `:80` is
   unavailable).

   > **Calling an HTTPS API from your app?** Every image ships the Mozilla
   > CA bundle at the standard system path, so it just works — see
   > [`docs/runtime.md`](docs/runtime.md#https-calls-and-the-ca-bundle).

   Need different source on a device, or per board (different pins, an
   optional peripheral)? `gosd build` passes your app's compile a `gosd`
   build tag plus each selected board's own — see
   [gating app source with build tags](docs/board-build-tags.md).

3. Build an image for your board:

   ```sh
   gosd build . --board pi-zero-2w -o hello.img
   ```

   Omit `--board` to build every supported board at once; `gosd build
   --help` lists the full flag set (`--boot-config-dir`, `-o`/`--output`,
   repeatable `--with-external` — see
   [`docs/runtime.md`](docs/runtime.md#bundling-a-companion-binary---with-external)).
   Every flag can also live in a checked-in `gosd-build.toml`, so a bare
   `gosd build` in a fresh checkout reproduces your repo's canonical
   image — see [checked-in build options](docs/build-config.md).

   No board on hand yet? `gosd run .` cross-compiles your app, builds an
   image, and boots it under `qemu-system-aarch64` in one step, so you can
   watch the real boot sequence and hit your app's HTTP port locally before
   ever touching hardware — see
   [`docs/runtime.md`](docs/runtime.md#testing-your-app-under-qemu-no-hardware-needed).

4. Flash `hello.img` to an SD card and boot it. The recommended path is
   [Raspberry Pi Imager](https://www.raspberrypi.com/software/)'s custom
   repository: build with `--publish-catalog --publish-base-url=<url>`, host the
   emitted `os_list.json` next to your image, and paste that URL into
   Imager's Settings → Custom repository — flashers get the full
   WiFi/hostname wizard. [`docs/publishing.md`](docs/publishing.md) is the
   developer walkthrough; [`docs/flashing.md`](docs/flashing.md) is a
   screenshot-driven, jargon-free version of the end-user steps you can
   send to non-technical people directly.

   (Imager's plain "Use custom image" file picker skips that wizard for
   *every* image, GoSD's included — see
   [`docs/provisioning-formats.md`](docs/provisioning-formats.md). If you
   flash that way, hand-edit the config tree on the flashed boot partition
   instead.)

   Then open `http://<hostname>.local/` — the hostname defaults to your
   main package's sanitized name.
   `gosd-init` runs its own mDNS responder, so `.local` resolves on macOS,
   Linux, and Windows with no extra setup; if your network blocks mDNS,
   find the device's address via your router.

## Going further

- **Caching** — downloaded board artifacts, the CA bundle, and any
  `--ingress` binary are cached under your OS user cache dir (e.g.
  `~/Library/Caches/gosd` on macOS, `~/.cache/gosd` on Linux) so repeat
  builds work offline; a successful build automatically prunes anything left
  over from an older gosd version or pin, so the cache holds only the
  current version's assets instead of growing forever. `gosd cache
  dir`/`size`/`clean` inspect or manually clear these (and, with
  `--builds`, the separate `build-kernel`/`build-external` cache):
  [`docs/artifacts.md`](docs/artifacts.md)
- **The runtime contract** your app runs under once booted — supervision,
  environment variables, networking timing, storage, logging, and what
  survives an upgrade: [`docs/runtime.md`](docs/runtime.md)
- **Crash reports on the card** — an unattended device with no screen and no
  console still has to tell someone what went wrong. On a fatal error it
  writes `LAST_FATAL_ERROR.md` to the root of its boot partition, readable on
  any computer and safe to forward: your app's own environment values are
  scrubbed out of it. The `fault` package lets your app raise one for a
  problem only a person can fix — and prints exactly the same report to your
  terminal when you run it off a device, so you can read what your user would
  read without flashing a card:
  [when to raise one, and what it says](docs/crash-reports.md)
- **A status LED shows boot state at a glance** — the LED marked ACT, the
  activity/status LED, the green LED, or whichever LED your board has,
  flashes evenly while booting, blips once a second while your app runs, and
  goes solid on for a recorded fatal error. No code changes, no config:
  [which LED gets used and why](docs/status-led.md)
- **The device's config tree** — the hand-editable settings on every
  card, one plain-text file per setting: [`docs/config.md`](docs/config.md)
- **Checked-in build options** (`gosd-build.toml`) — record the options your
  app's image needs (boards, partition sizes, an ingress tunnel, placeholder
  files) in a file next to your code: every key is a `gosd build` flag of
  the same name, a bare `gosd build` reproduces the repo's canonical image,
  and a flag on the command line still wins per option:
  [checked-in build options](docs/build-config.md)
- **Custom kernels** (`gosd build-kernel`) — need a driver GoSD's stock,
  trimmed kernels cut (a USB DVB-T tuner, a niche sensor)? An opt-in,
  Docker/Podman-driven command compiles one from a `gosd-kernel.toml` in
  your project; the default build path stays zero-Docker for everyone else:
  [`docs/custom-kernels.md`](docs/custom-kernels.md)
- **Sound** — one of those cut drivers. With a custom kernel, the `sound`
  package plays PCM out of HDMI or a board's headphone jack, no cgo and no
  alsa-lib. Per-board recipes and the traps: [`docs/sound.md`](docs/sound.md);
  `examples/chime` is the worked example.
- **Companion binaries** (`gosd build-external`) — need something that isn't
  pure Go (a hardware-accelerated video player, a vendor CLI)? The same kind
  of opt-in, Docker/Podman-driven command cross-compiles a fully static
  binary from a `gosd-external.toml` recipe, and `gosd build
  --with-external` bundles it into the image:
  [`docs/externals.md`](docs/externals.md)
- **Injecting per-user config after the image is built**
  (`gosd build --placeholder <path>=<size>`) — distributing a per-deployment
  secret or identity (an API key, a device's WiFi credentials) without
  building a different image per recipient? Reserve a placeholder file at
  build time and a downstream tool (typically a browser, between the CDN
  and the user's disk) can splice real content into the declared byte
  ranges with no FAT32 code at all: [`docs/image-injection.md`](docs/image-injection.md).
  The official `@jphastings/gosd` npm package does this end to end in one call —
  `import { withPlaceholders } from "@jphastings/gosd/downloads"` — see
  [`js/packages/gosd/README.md`](js/packages/gosd/README.md)
