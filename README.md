# GoSD

Turn Go applications into SD card images for the Raspberry Pi Zero 2w and the Radxa Zero 3E. (Planned: FriendlyElec NanoPi Zero2.)

Like GoKrazy, but the result is something _anyone_ can burn and use.

## Features

- Simple CLI tool that can be run locally or in CI
- Extremely fast boot (under 5 seconds, including Wifi)
- Optional USB OTG (run as a USB _device_)
- Connect to the internet via Ethernet (assumes DHCP) or WiFi (credentials added as your SD card is written)
- Run any normal (linux-capable) Go application

## Quickstart

> **This project is pre-release.** The steps below are the intended
> end-to-end workflow, written against what exists on `main` today. Two
> things stop it from being runnable start-to-finish right now:
>
> - No version of `gosd` has been tagged/released yet, so `go install
>   .../gosd@latest` has nothing to install.
> - `gosd build` fully assembles a flashable `.img` given `--artifacts-dir`
>   (a directory of your own kernel/firmware/bootloader files), but no
>   `artifacts/vX.Y.Z` release exists yet for it to download prebuilt
>   kernels/U-Boot from automatically (see `docs/artifacts.md` and bean
>   `gosd-wtpa`) — that's one `git tag artifacts/v0.1.0 && git push` away.
>   Until then, running `gosd build` with no `--artifacts-dir` fails
>   clearly at the download step instead of silently succeeding.
>
> Once that release is cut, the flow below is what you'll run. In the
> meantime, see `internal/build`, `examples/hello`, and `docs/runtime.md`
> for what's real today.

1. Install the CLI:

   ```sh
   go install github.com/jphastings/gosd/cmd/gosd@latest
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

3. Build an image for your board:

   ```sh
   gosd build . --board pi-zero-2w -o hello.img
   ```

   Omit `--board` to build every supported board at once; run `gosd build
   --help` for the full set of flags (`--hostname`, `--wifi-ssid` /
   `--wifi-pass`, repeatable `--board`, `-o`/`--output`).

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
   your main package if you didn't pass `--hostname`. A dedicated
   screenshot-driven flashing guide for non-technical users is planned
   (bean `gosd-ufeh`); until then, and until mDNS support lands (bean
   `gosd-r796`), you may need to find the device's address via your
   router instead of `.local`.

For the runtime contract your app runs under once it's booted — supervision,
environment variables, networking timing, storage, logging — see
[`docs/runtime.md`](docs/runtime.md).
