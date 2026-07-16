# GoSD

Turn Go applications into SD card images for the Raspberry Pi Zero 2W, Raspberry Pi Zero W, Radxa Zero 3E, and FriendlyElec NanoPi Zero2. See [`COMPATIBILITY.md`](COMPATIBILITY.md) for exactly which features work on which board today.

Like GoKrazy, but the result is something _anyone_ can burn and use.

## Features

- Simple CLI tool that can be run locally or in CI
- Extremely fast boot (under 5 seconds, including Wifi)
- Optional USB OTG (run as a USB _device_) — see [`docs/runtime.md`](docs/runtime.md#usb-gadget-mode) and `examples/usbserial`
- Connect to the internet via Ethernet (assumes DHCP) or WiFi (credentials added as your SD card is written)
- Run any normal (linux-capable) Go application

## Quickstart

> **This project is pre-release**, but the steps below are the real,
> working pipeline as of `main` today — not aspirational. `go install
> .../gosd@latest` installs cleanly even though no numbered CLI release
> has been tagged yet (Go resolves `@latest` to the newest commit when
> there's no tag to pin to). `gosd build` with no extra flags downloads the
> published `artifacts/v0.1.0` kernels/bootloader from GitHub Releases
> (see `docs/artifacts.md`), cross-compiles your app, and assembles a
> complete, flashable `.img` — verified end-to-end on a clean machine
> (empty cache, and an offline rebuild afterwards to confirm the cache
> alone is sufficient).
>
> What's genuinely still missing is hardware bring-up: no GoSD image has
> yet been flashed to and booted from a real Pi Zero 2W or Radxa Zero 3E
> (tracked by beans `gosd-m9dj`/`gosd-nlzf`). Everything through "produce
> `hello.img`" below is real and tested; the flashing and booting steps are
> the intended flow, not yet confirmed on physical hardware. See
> [`COMPATIBILITY.md`](COMPATIBILITY.md) for the full board/feature
> breakdown.

1. Install the CLI:

   ```sh
   go install github.com/jphastings/gosd/cmd/gosd@latest
   ```

   Or, with nix (handy in CI — the flake bundles the Go toolchain and a
   vendored copy of gosd's own sources, so `gosd build` works offline
   apart from your app's dependencies and board artifacts):

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

   > **Calling an HTTPS API from your app?** GoSD images ship no CA bundle,
   > so `crypto/x509` has no roots to verify a server's certificate against
   > and outbound HTTPS fails until you blank-import
   > `golang.org/x/crypto/x509roots/fallback` — see
   > [`docs/runtime.md`](docs/runtime.md#https-calls-need-a-ca-bundle-your-app-supplies)
   > for the full pattern.

   Need different source per board (different pins, an optional
   peripheral)? `gosd build` passes each selected board's own Go build tag
   to your app's compile — see
   [`docs/board-build-tags.md`](docs/board-build-tags.md).

3. Build an image for your board:

   ```sh
   gosd build . --board pi-zero-2w -o hello.img
   ```

   Omit `--board` to build every supported board at once; run `gosd build
   --help` for the full set of flags (`--hostname`, `--wifi-ssid` /
   `--wifi-pass`, repeatable `--board`, `-o`/`--output`, repeatable
   `--with-external` to bundle a prebuilt static companion binary — see
   [`docs/runtime.md`](docs/runtime.md#bundling-a-companion-binary---with-external)).

   Don't have a board on hand yet? `gosd run .` cross-compiles your app,
   builds an image, and boots it under `qemu-system-aarch64` in one step,
   so you can see the real boot sequence and hit your app's HTTP port
   locally before ever touching hardware — see
   [`docs/runtime.md`](docs/runtime.md#testing-your-app-under-qemu-no-hardware-needed).

4. Flash `hello.img` to an SD card. The recommended path is [Raspberry Pi
   Imager](https://www.raspberrypi.com/software/)'s custom-repository
   catalog: build with `--catalog --publish-base-url=<url>` to also emit an
   `os_list.json`, host it next to your image, and paste that URL into
   Imager's Settings → Custom repository to get the full WiFi/hostname
   customization wizard — see [`docs/publishing.md`](docs/publishing.md)
   for the full walkthrough. (Imager's plain "Use custom image" file picker
   skips that wizard entirely for any image, GoSD's included — see
   `docs/provisioning-formats.md` — so if you use that flow instead,
   hand-edit the `gosd.toml` file on the flashed boot partition.) Then boot
   the board and open `http://<hostname>.local/` — or the sanitized name of
   your main package if you didn't pass `--hostname`. `gosd-init` runs its
   own mDNS responder, so `.local` should resolve on macOS, Linux, and
   Windows without any extra setup; if it doesn't on your network, fall
   back to finding the device's address via your router. Sending an app to
   someone non-technical? [`docs/flashing.md`](docs/flashing.md) is a
   screenshot-driven, jargon-free version of these same steps you can point
   them at directly.

For the runtime contract your app runs under once it's booted — supervision,
environment variables, networking timing, storage, logging — see
[`docs/runtime.md`](docs/runtime.md).

Need a driver GoSD's stock, trimmed kernels cut (a USB DVB-T tuner, a niche
sensor)? `gosd build-kernel` is an opt-in, Docker/Podman-driven command that
compiles a custom kernel from a `gosd-kernel.toml` you declare in your
project, without slowing down the default zero-Docker path for everyone
else — see [`docs/custom-kernels.md`](docs/custom-kernels.md).

Need a companion binary that isn't pure Go (a hardware-accelerated video
player, a vendor CLI)? `gosd build-external` is the same kind of opt-in,
Docker/Podman-driven command, cross-compiling one from a
`gosd-external.toml` recipe into a fully static binary `gosd build
--with-external` bundles into the image — see
[`docs/externals.md`](docs/externals.md).
